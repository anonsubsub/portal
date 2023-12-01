package simulation

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/consensys/gnark-crypto/accumulator/merkletree"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/hash"
	"github.com/consensys/gnark/backend/plonk"
	plonk_bn254 "github.com/consensys/gnark/backend/plonk/bn254"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/scs"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go-sdk/circuits"
	"go-sdk/circuits/merklecombined"
	"go-sdk/circuits/mimc"
	"go-sdk/client"
	"go-sdk/contracts/MerkleCombined"
	"go-sdk/contracts/MimcCombined"
	"go-sdk/deployment"
	"log"
	"math/big"
	"math/rand"
	"strconv"
	"time"
)

var totalGasUsage uint64 = 0
var gasPrice = big.NewInt(2800000000)
var solidityPath = "circuits/contracts/%s.sol"
var SNARK_SCALAR_FIELD, _ = new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

const totalClaim = 100

var circuitMap = map[string]interface{}{
	"MerkleCombined": merklecombined.MerkleCombined{},
	"MimcCombined":   mimc.MimcCombined{},
}

func SimulateIpfsOnChainPublicClaim(registryAddr string, userAccount common.Address, userSigner *client.Signer,
	reader *client.TxReader, ipfsConn string) {

	fmt.Println("Simulation type: ipfs-on-chain")
	regClient := client.NewRegistryClient(registryAddr, *reader)
	manager, err := client.GetManager(regClient, userAccount)
	if err != nil {
		fmt.Println("Error getting manager:", err)
		return
	}
	log.Printf("Manager address for user %s: %s\n", userAccount.Hex(), manager)

	manClient := client.NewManagerClient(manager.Hex(), *reader, ipfsConn)
	// start timer
	startTime := time.Now()
	for i := 0; i < totalClaim; i++ {
		jsonStr := GenerateDummyClaim()
		key := strconv.Itoa(i)
		rec, err := client.PublishClaim(manClient, userSigner, key, jsonStr)
		totalGasUsage += rec.GasUsed

		if err != nil {
			log.Fatal("Could not set claim", err)
			return
		}
	}

	// Calculate the execution time.
	elapsedTime := time.Since(startTime)

	fmt.Printf("Number of %d claims to set took %s\n.Average per claim:%s \n", totalClaim, elapsedTime, elapsedTime/totalClaim)

	fmt.Printf("Transaction Cost (Wei): %d \n", totalGasUsage)
	fmt.Printf("Transaction Cost (Ether): %s\n", weiToEther(totalGasUsage).String())
	total_cost := new(big.Int)
	total_gas := new(big.Int).SetUint64(totalGasUsage)
	total_cost.Mul(total_gas, gasPrice)
	fmt.Printf("Total Cost (Ether): %s\n", weiToEther(total_cost.Uint64()).String())
}

func SimulateOnChainPublicClaims(registryAddr string, userAccount common.Address, userSigner *client.Signer,
	reader *client.TxReader) {
	fmt.Println("Simulation type: onchain")
	regClient := client.NewRegistryClient(registryAddr, *reader)
	// check identity is registered
	manager, err := client.GetManager(regClient, userAccount)
	if err != nil {
		fmt.Println("Error getting manager:", err)
		return
	}
	log.Printf("Manager address for user %s: %s\n", userAccount.Hex(), manager)

	// now user can set public claim by it's own manager
	manClient := client.NewManagerClient(manager.Hex(), *reader, "")
	// start timer
	startTime := time.Now()
	for i := 0; i < totalClaim; i++ {
		jsonStr := GenerateDummyClaim()
		key := strconv.Itoa(i)
		rec, err := client.SetPublicClaim(manClient, userSigner, key, string(jsonStr), "age > 22")
		totalGasUsage += rec.GasUsed

		if err != nil {
			log.Fatal("Could not set claim", err)
			return
		}

	}

	// Calculate the execution time.
	elapsedTime := time.Since(startTime)

	fmt.Printf("Number of %d claims to set took %s\n.Average per claim:%s \n", totalClaim, elapsedTime, elapsedTime/totalClaim)

	fmt.Printf("Transaction Cost (Wei): %d \n", totalGasUsage)
	fmt.Printf("Transaction Cost (Ether): %s\n", weiToEther(totalGasUsage).String())
	total_cost := new(big.Int)
	total_gas := new(big.Int).SetUint64(totalGasUsage)
	total_cost.Mul(total_gas, gasPrice)
	fmt.Printf("Total Cost (Ether): %s\n", weiToEther(total_cost.Uint64()).String())
}

/*
Tests setting public claim on ipfs only.
No contract interaction is involved.
*/
func SimulateIPFSConnection(ipfsConn string) {
	fmt.Println("Simulation type: ipfs")

	cli := client.NewIpfsClient(ipfsConn)
	// start timer
	startTime := time.Now()
	for i := 0; i < totalClaim; i++ {
		jsonStr := GenerateDummyClaim()

		_, err := cli.AddAndPublish(jsonStr)

		if err != nil {
			log.Fatal("Could not set claim", err)
			return
		}

	}

	elapsedTime := time.Since(startTime)

	fmt.Printf("Number of %d claims to set took %s\n.Average per claim:%s \n",
		totalClaim, elapsedTime, elapsedTime/totalClaim)
}

func SimulateContractDeployment(userSigner *client.Signer, ownerSigner *client.Signer, reader *client.TxReader) {

	fmt.Println("Simulation type: deployment")

	// ownerAccount deploys the registry
	startTime := time.Now()
	registryContract, tx := deployment.DeployRegistry(ownerSigner)
	elapsedTime := time.Since(startTime)

	fmt.Println("Registry contract deployed at:", registryContract)
	fmt.Printf("Deployment time: %s\n", elapsedTime)

	receipt, err := ownerSigner.WaitForReceipt(tx.Hash())
	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
	}
	calculateGasUsage("Registry deployment", receipt)

	bytecode, err := userSigner.Client.CodeAt(context.Background(), registryContract, nil) // nil is latest block
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytecode length for registry contract: %d\n", len(bytecode))

	// start timer
	startTime = time.Now()

	// userAccount deploys it's own manager
	managerAddr, tx := deployment.DeployManager(userSigner, registryContract)
	elapsedTime = time.Since(startTime)

	receipt, err = userSigner.WaitForReceipt(tx.Hash())
	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
	}
	fmt.Println("User identity contract deployed at:", managerAddr)
	fmt.Printf("Deployment time: %s\n", elapsedTime)
	calculateGasUsage("Manager deployment", receipt)

	bytecode, err = userSigner.Client.CodeAt(context.Background(), managerAddr, nil) // nil is latest block
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytecode length for user identity contract: %d\n", len(bytecode))

	// attestor deploys it's own manager
	attestorIdentityAddr, tx := deployment.DeployManager(ownerSigner, registryContract)
	elapsedTime = time.Since(startTime)

	receipt, err = ownerSigner.WaitForReceipt(tx.Hash())
	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
	}
	fmt.Println("Attestor identity contract deployed at:", managerAddr)
	fmt.Printf("Deployment time: %s\n", elapsedTime)
	calculateGasUsage("Manager deployment", receipt)

	bytecode, err = userSigner.Client.CodeAt(context.Background(), managerAddr, nil) // nil is latest block
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytecode length for attestor identity contract: %d\n", len(bytecode))

	regClient := client.NewRegistryClient(registryContract.Hex(), *reader)
	receipt, err = client.RegisterIdentity(regClient, userSigner, ownerSigner, managerAddr)
	if err != nil {
		log.Fatal("Could not register identity", err)
	}
	receipt, err = userSigner.WaitForReceipt(receipt.TxHash)
	if err != nil {
		return
	}

	fmt.Println("Identity registered for user:", common.Bytes2Hex(userSigner.PublicKey))
	receipt, err = client.RegisterIdentity(regClient, ownerSigner, ownerSigner, attestorIdentityAddr)
	receipt, err = userSigner.WaitForReceipt(receipt.TxHash)
	if err != nil {
		return
	}
	fmt.Println("Identity registered for attestor:", common.Bytes2Hex(ownerSigner.PublicKey))

}

func SimulateMerkleCircuitDeployment(registryOwner *client.Signer, ipfsConn string, registryAddr string, reader *client.TxReader) {
	fmt.Println("Simulation type: circuit build")
	fmt.Println("Pre-required simulations: deploy")
	circuitName := "MerkleCombined"
	ipfsClient := client.NewIpfsClient(ipfsConn)
	// first build the circuit
	fmt.Println("Simulation type: circuit build")
	curve := ecc.BN254
	numLeaves := 32
	segmentSize := 32
	mod := curve.ScalarField()
	modNbBytes := len(mod.Bytes())
	depth := 5
	var merkleCircuit merklecombined.MerkleCombined
	merkleCircuit.Path = make([]frontend.Variable, depth+1)

	cc, err := frontend.Compile(curve.ScalarField(), scs.NewBuilder, &merkleCircuit)
	if err != nil {
		log.Fatal(err)
	}

	srs, err := test.NewKZGSRS(cc)
	if err != nil {
		log.Fatal("test.NewKZGSRS(ccs)")
	}
	// groth16 zkSNARK: Setup
	pk, vk, err := plonk.Setup(cc, srs)
	if err != nil {
		log.Fatal("plonk.Setup", err)
	}
	path := fmt.Sprintf(solidityPath, circuitName)
	deployment.CircuitToSolidity(vk, path)
	deployment.CreateCircuitBindings("circuits/contracts", circuitName)
	startTime := time.Now()
	circuitAddr, tx := deployment.DeployMerkleCircuit(registryOwner)
	elapsedTime := time.Since(startTime)

	receipt, err := registryOwner.WaitForReceipt(tx.Hash())
	fmt.Printf("Merklecircuit deployment time: %s\n", elapsedTime)
	fmt.Printf("CircuitContract deployed at: %s\n", circuitAddr)
	calculateGasUsage("MerkleCombined deployment", receipt)

	bytecode, err := registryOwner.Client.CodeAt(context.Background(), circuitAddr, nil) // nil is latest block
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytecode len: %d\n", len(bytecode))

	var (
		pre       = 100
		threshold = 22
		digest    = "20370067689261511688289967978544823130432235585709842144916192060767982363628"
	)

	startTime = time.Now()

	var buf bytes.Buffer
	mimcGo := hash.MIMC_BN254.New()
	// convert pre to bytes with padding to 32 bytes
	inpBytes := make([]byte, 32)
	preBytes := []byte(strconv.Itoa(pre))
	copy(inpBytes[32-len(preBytes):], preBytes)

	mimcGo.Write(inpBytes)
	sum := mimcGo.Sum(nil)

	// normalize the input with mod BN254
	inputMod := new(big.Int).SetBytes(sum)
	inputMod.Mod(inputMod, curve.BaseField())

	sumModBytes := inputMod.Bytes()
	buf.Write(make([]byte, modNbBytes-len(sumModBytes)))
	buf.Write(sumModBytes)

	//// some random elements for tree
	for i := 1; i < numLeaves; i++ {
		leaf, _ := crand.Int(crand.Reader, mod)

		b := leaf.Bytes()

		buf.Write(make([]byte, modNbBytes-len(b)))
		buf.Write(b)
	}

	hGo := hash.MIMC_BN254.New()

	//merkleRoot, proofPath, index, leaves := tree.Prove()
	merkleRoot, proofPath, _, err := merkletree.BuildReaderProof(&buf, hGo, segmentSize, uint64(0))

	if err != nil {
		log.Fatal(err)
	}

	// wt
	var merkleWt merklecombined.MerkleCombined
	merkleWt.Input = pre
	merkleWt.Threshold = threshold
	merkleWt.Digest = digest
	merkleWt.RootHash = merkleRoot
	merkleWt.Nonce = 0
	merkleWt.NoncePriv = 0
	merkleWt.Sender = 0
	merkleWt.SenderPriv = 0
	merkleWt.Leaf = 0
	merkleWt.Path = make([]frontend.Variable, depth+1)
	for i := 0; i < depth+1; i++ {
		merkleWt.Path[i] = proofPath[i]
	}

	w, err := frontend.NewWitness(&merkleWt, curve.ScalarField())
	if err != nil {
		log.Fatal("Couldn't set witness ", err)
	}

	//// groth16: Prove & Verify
	proof, err := plonk.Prove(cc, pk, w)
	if err != nil {
		log.Fatal("prove computation failed...", err)
	}

	//publicWit, _ := w.Public()
	//err = plonk.Verify(proof, vk, publicWit)
	//if err != nil {
	//	log.Fatal("Verify failed from local verifier", err)
	//	return
	//}

	serialized := circuits.Serialize(cc, srs, w, vk, pk, proof, "MerkleCombined", circuitAddr.Hex())
	ipfs, err := circuits.PublishCircuitOnIpfs(ipfsClient, &serialized)
	if err != nil {
		log.Fatal("Could not publish circuit on ipfs", err)
		return
	}
	log.Printf("CircuitMeta published on ipfs: %s\n", ipfs)
	elapsedTime = time.Since(startTime)
	fmt.Printf("CircuitMeta build & publish time: %s\n", elapsedTime)
	fmt.Println("Simulation type: deployment of circuit")

	//register circuit metadata on registry
	regClient := client.NewRegistryClient(registryAddr, *reader)
	startTime = time.Now()
	cMeta := client.CircuitMeta{
		DeploymentType: "ipfs",
		Address:        circuitAddr,
		IpfsURI:        ipfs,
	}
	setReceipt, err := client.RegisterCircuit(regClient, registryOwner, "MerkleCombined", &cMeta)
	if err != nil {
		log.Fatal("Could not register circuit metadata on chain", err)
		return
	}
	elapsedTime = time.Since(startTime)
	// get total gas used
	calculateGasUsage("SetCircuit in registry", setReceipt)
	fmt.Printf("SetCircuit in registry time: %s\n", elapsedTime)
}

func PrepareCircuitForDeployment(circuitName string) {
	curve := ecc.BN254
	var mimcCircuit mimc.MimcCombined
	ccs, err := frontend.Compile(curve.ScalarField(), scs.NewBuilder, &mimcCircuit)
	if err != nil {
		log.Fatal(err)
	}
	srs, err := test.NewKZGSRS(ccs)
	if err != nil {
		log.Fatal("test.NewKZGSRS(ccs)", err)
	}
	// groth16 zkSNARK: Setup
	_, vk, err := plonk.Setup(ccs, srs)
	if err != nil {
		log.Fatal("plonk.Setup", err)
	}
	path := fmt.Sprintf(solidityPath, circuitName)
	deployment.CircuitToSolidity(vk, path)

}
func SimulateMimcCircuitDeployment(registryOwner *client.Signer, ipfsConn string, registryAddr string, reader *client.TxReader) {
	fmt.Println("Simulation type: circuit build")
	fmt.Println("Pre-required simulations: deploy")

	// 1. build the circuit and deploy the verifier contract
	circuitName := "MimcCombined"
	ipfsClient := client.NewIpfsClient(ipfsConn)
	regClient := client.NewRegistryClient(registryAddr, *reader)
	curve := ecc.BN254

	startTime := time.Now()
	var mimcCircuit mimc.MimcCombined

	cc, err := frontend.Compile(curve.ScalarField(), scs.NewBuilder, &mimcCircuit)
	if err != nil {
		log.Fatal(err)
	}

	srs, err := test.NewKZGSRS(cc)
	if err != nil {
		log.Fatal("test.NewKZGSRS(ccs)", err)
	}
	// groth16 zkSNARK: Setup
	pk, vk, err := plonk.Setup(cc, srs)
	if err != nil {
		log.Fatal("plonk.Setup", err)
	}
	path := fmt.Sprintf(solidityPath, circuitName)
	deployment.CircuitToSolidity(vk, path)
	deployment.CreateCircuitBindings("circuits/contracts", circuitName)
	startTime = time.Now()
	circuitAddr, tx := deployment.DeployMimcCircuit(registryOwner)
	elapsedTime := time.Since(startTime)
	//
	fmt.Printf("CircuitContract deployed at: %s\n", circuitAddr)
	receipt, err := registryOwner.WaitForReceipt(tx.Hash())

	//wt
	var (
		pre       = 100
		threshold = 22
		digest    = "20370067689261511688289967978544823130432235585709842144916192060767982363628"
	)
	var mimcWt mimc.MimcCombined
	mimcWt.Input = pre
	mimcWt.Threshold = threshold
	mimcWt.Digest = digest
	mimcWt.Nonce = 0
	mimcWt.NoncePriv = 0
	mimcWt.Sender = registryOwner.PublicKey
	mimcWt.SenderPriv = registryOwner.PublicKey
	w, err := frontend.NewWitness(&mimcWt, curve.ScalarField())
	if err != nil {
		log.Fatal("Couldn't set witness ", err)
	}
	proof, err := plonk.Prove(cc, pk, w)
	if err != nil {
		log.Fatal("prove computation failed...", err)
	}

	// 2. publish circuit metadata on ipfs
	startTime = time.Now()

	serialized := circuits.Serialize(cc, srs, w, vk, pk, proof, circuitName, circuitAddr.Hex())
	ipfs, err := circuits.PublishCircuitOnIpfs(ipfsClient, &serialized)
	if err != nil {
		log.Fatal("Could not publish circuit on ipfs", err)
		return
	}
	log.Printf("CircuitMeta published on ipfs: %s\n", ipfs)
	elapsedTime = time.Since(startTime)
	fmt.Printf("CircuitMeta build & publish time: %s\n", elapsedTime)
	cMeta := client.CircuitMeta{
		DeploymentType: "ipfs",
		Address:        circuitAddr,
		IpfsURI:        ipfs,
	}

	//3. Register circuit on chain
	setReceipt, err := client.RegisterCircuit(regClient, registryOwner, circuitName, &cMeta)
	if err != nil {
		log.Fatal("Could not register circuit metadata on chain", err)
		return
	}
	elapsedTime = time.Since(startTime)

	receipt, err = registryOwner.WaitForReceipt(setReceipt.TxHash)
	if err != nil {
		return
	}
	calculateGasUsage("SetCircuit in registry", receipt)

	log.Printf("CircuitMeta published on ipfs: %s\n", ipfs)
	elapsedTime = time.Since(startTime)
	fmt.Printf("CircuitMeta build & publish time: %s\n", elapsedTime)
	fmt.Println("Simulation type: deployment of circuit")

	// get transaction receipt
	receipt, err = registryOwner.WaitForReceipt(tx.Hash())

	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
	}
	calculateGasUsage("CircuitMeta deployment", receipt)

	bytecode, err := registryOwner.Client.CodeAt(context.Background(), circuitAddr, nil) // nil is latest block
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytecode len: %d\n", len(bytecode))

	startTime = time.Now()
	circuitData, _ := client.GetCircuit(regClient, circuitName)
	elapsedTime = time.Since(startTime)
	fmt.Printf("GetCircuit from registry time: %s\n", elapsedTime)
	fmt.Printf("CircuitMeta data from registry: %s\n", circuitData)
}

func SimulatePrivateClaim(userSigner *client.Signer, reader *client.TxReader,
	ipfsConn string, registryAddr string, userManagerAddr string, verifierCircuitName string) {
	fmt.Println("Simulation type: private claim")
	fmt.Println("Pre-required simulations: deploy,circuitDeployment")

	ipfsCli := client.NewIpfsClient(ipfsConn)

	// register circuit metadata on registry
	regClient := client.NewRegistryClient(registryAddr, *reader)
	manClient := client.NewManagerClient(userManagerAddr, *reader, ipfsConn)

	circuitMeta, err := client.GetCircuit(regClient, verifierCircuitName)
	if err != nil {
		log.Fatal("Could not get circuit metadata from registry", err)
		return
	}
	fmt.Printf("Circuit contract address: %s\n", circuitMeta.Address.Hex())
	//build circuit from metadata
	proofSystem := circuits.BuildCircuitFrom(circuitMeta.IpfsURI, ipfsCli)

	// generate private claim (proof) and deploy verifier circuit
	curve := ecc.BN254
	var (
		pre       = 100
		threshold = 22
		digest    = "20370067689261511688289967978544823130432235585709842144916192060767982363628"
	)

	var circuit mimc.MimcCombined
	circuit.SenderPriv = userSigner.PublicKey
	circuit.Sender = userSigner.PublicKey
	circuit.Input = pre
	circuit.Threshold = threshold
	circuit.Digest = digest

	// call contract to get nonce
	auth := userSigner.BindTxOpts()
	verifierContract, _ := MimcCombined.NewMimcCombined(circuitMeta.Address, userSigner.Client)

	nonceTx, err := verifierContract.SetNonce(auth)
	nonceRec, err := userSigner.WaitForReceipt(nonceTx.Hash())
	if err != nil {
		return
	}
	calculateGasUsage("SetNonce", nonceRec)

	callOpts := bind.CallOpts{}
	returnedNonce, err := verifierContract.Nonces(&callOpts, auth.From)
	if err != nil {
		log.Fatal("nonce transaction failed", err)
	}
	log.Println("nonce returned from contract:", returnedNonce[:len(returnedNonce)-1])

	// take mod of nonce and SNARK_SCALAR_FIELD
	nonceMod, _ := new(big.Int).SetString(returnedNonce[:len(returnedNonce)-1], 10)
	nonceMod.Mod(nonceMod, SNARK_SCALAR_FIELD)

	//nonce should be set from circuit nonce
	circuit.Nonce = nonceMod
	circuit.NoncePriv = nonceMod

	w, err := frontend.NewWitness(&circuit, curve.ScalarField())
	if err != nil {
		log.Fatal("Couldn't set witness ", err)
	}

	// plonk zkSNARK: Setup
	pk, _, err := plonk.Setup(proofSystem.Ccs, proofSystem.Srs)
	if err != nil {
		log.Fatal("plonk.Setup", err)
	}

	// plonk: generate proof from witness
	proof, err := plonk.Prove(proofSystem.Ccs, pk, w)
	if err != nil {
		log.Fatal("prove computation failed...", err)
	}
	// call contract verify function
	var (
		input [4]*big.Int
	)

	// public witness, the hash of the secret is on chain
	input[0] = new(big.Int).SetInt64(int64(threshold))
	input[1], _ = new(big.Int).SetString(digest, 10)
	input[2] = new(big.Int).SetBytes(userSigner.PublicKey)
	input[3] = nonceMod

	p := proof.(*plonk_bn254.Proof)
	serializedProof := p.MarshalSolidity()
	// call the contract
	log.Println("on-chain verifying proof with input:", input[0].String())
	log.Println("on-chain verifying proof with nonce:", nonceMod.String())

	auth = userSigner.BindTxOpts()
	tx, err := verifierContract.Verify(auth, serializedProof[:], input[:])
	if err != nil {
		log.Fatal("transaction failed ", err)
		return
	}

	// get the transaction receipt
	receipt, err := userSigner.WaitForReceipt(tx.Hash())
	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
		return
	}

	// check if the transaction was successful
	if receipt.Status != 1 {
		log.Fatal("transaction failed", receipt.Status)
		return
	}

	var buf bytes.Buffer
	_, err = proof.WriteTo(&buf)
	if err != nil {
		return
	}
	// start timer
	startTime := time.Now()

	receipt, err = client.SetPrivateClaim(manClient, userSigner, "agePriv", buf.String(),
		"age > 22", circuitMeta.IpfsURI, receipt.Logs[0].Topics[0].String())
	elapsedTime := time.Since(startTime)
	fmt.Printf("SetPrivateClaim took: %s\n", elapsedTime)
	if err != nil {
		return
	}
	waitForReceipt, err := userSigner.WaitForReceipt(receipt.TxHash)
	if err != nil {
		return
	}
	calculateGasUsage("SetPrivateClaim", waitForReceipt)

	// read private claim on chain
	startTime = time.Now()
	claim, err := client.GetPrivateClaim(manClient, "agePriv")
	elapsedTime = time.Since(startTime)
	fmt.Printf("GetPrivateClaim took: %s\n", elapsedTime)
	fmt.Printf("Private claim: id:%d statement:%s eventHash: %s\n", claim.ClaimMeta.Id,
		claim.ClaimMeta.Statement,
		claim.EventHash)

}
func SimulatePrivateMerkleClaim(userSigner *client.Signer, reader *client.TxReader,
	ipfsConn string, registryAddr string, userManagerAddr string, verifierCircuitName string) {
	fmt.Println("Simulation type: private claim")
	fmt.Println("Pre-required simulations: deploy,merkleDeployment")

	ipfsCli := client.NewIpfsClient(ipfsConn)

	// register circuit metadata on registry
	regClient := client.NewRegistryClient(registryAddr, *reader)
	manClient := client.NewManagerClient(userManagerAddr, *reader, ipfsConn)

	circuitMeta, err := client.GetCircuit(regClient, verifierCircuitName)
	if err != nil {
		log.Fatal("Could not get circuit metadata from registry", err)
		return
	}
	fmt.Printf("Circuit contract address: %s\n", circuitMeta.Address.Hex())
	//build circuit from metadata
	proofSystem := circuits.BuildCircuitFrom(circuitMeta.IpfsURI, ipfsCli)

	// generate private claim (proof) and deploy verifier circuit
	var (
		pre       = 100
		threshold = 22
		digest    = "20370067689261511688289967978544823130432235585709842144916192060767982363628"
		depth     = 5
	)
	curve := ecc.BN254
	numLeaves := 32
	segmentSize := 32
	mod := curve.ScalarField()
	modNbBytes := len(mod.Bytes())

	var circuit merklecombined.MerkleCombined
	circuit.Input = pre
	circuit.Threshold = threshold
	circuit.Digest = digest
	circuit.Leaf = 0
	circuit.Path = make([]frontend.Variable, depth+1)

	circuit.SenderPriv = userSigner.PublicKey
	circuit.Sender = userSigner.PublicKey

	// call contract to get nonce
	auth := userSigner.BindTxOpts()
	verifierContract, _ := MerkleCombined.NewMerkleCombined(circuitMeta.Address, userSigner.Client)

	nonceTx, err := verifierContract.SetNonce(auth)
	nonceRec, err := userSigner.WaitForReceipt(nonceTx.Hash())
	if err != nil {
		return
	}
	calculateGasUsage("SetNonce", nonceRec)

	callOpts := bind.CallOpts{}
	returnedNonce, err := verifierContract.Nonces(&callOpts, auth.From)
	if err != nil {
		log.Fatal("nonce transaction failed", err)
	}
	log.Println("nonce returned from contract:", returnedNonce[:len(returnedNonce)-1])

	// take mod of nonce and SNARK_SCALAR_FIELD
	nonceMod, _ := new(big.Int).SetString(returnedNonce[:len(returnedNonce)-1], 10)
	nonceMod.Mod(nonceMod, SNARK_SCALAR_FIELD)

	// generate merkle tree

	var buf bytes.Buffer
	mimcGo := hash.MIMC_BN254.New()

	// convert pre to bytes with padding to 32 bytes
	inpBytes := make([]byte, 32)
	preBytes := []byte(strconv.Itoa(pre))
	copy(inpBytes[32-len(preBytes):], preBytes)

	mimcGo.Write(inpBytes)
	sum := mimcGo.Sum(nil)
	// normalize the input with mod BN254
	inputMod := new(big.Int).SetBytes(sum)
	inputMod.Mod(inputMod, curve.BaseField())

	sumModBytes := inputMod.Bytes()
	buf.Write(make([]byte, modNbBytes-len(sumModBytes)))
	buf.Write(sumModBytes)

	//// some random elements for tree
	for i := 1; i < numLeaves; i++ {
		leaf, _ := crand.Int(crand.Reader, mod)

		b := leaf.Bytes()

		buf.Write(make([]byte, modNbBytes-len(b)))
		buf.Write(b)
	}

	hGo := hash.MIMC_BN254.New()

	//merkleRoot, proofPath, index, leaves := tree.Prove()
	merkleRoot, proofPath, leaves, err := merkletree.BuildReaderProof(&buf, hGo, segmentSize, uint64(0))
	merkleInt := new(big.Int).SetBytes(merkleRoot)
	if err != nil {
		log.Fatal(err)
	}
	buf.Reset()
	// verify the proof in plain go (data part)
	verified := merkletree.VerifyProof(hGo, merkleRoot, proofPath, 0, leaves)
	if !verified {
		log.Print("The merkle proof in plain go should pass", err)
	}

	//nonce should be set from circuit nonce
	circuit.RootHash = merkleRoot
	circuit.Nonce = nonceMod
	circuit.Sender = userSigner.PublicKey
	circuit.SenderPriv = userSigner.PublicKey
	circuit.NoncePriv = nonceMod
	for i := 0; i < depth+1; i++ {
		circuit.Path[i] = proofPath[i]
	}

	w, err := frontend.NewWitness(&circuit, curve.ScalarField())
	if err != nil {
		log.Fatal("Couldn't set witness ", err)
	}

	// plonk zkSNARK: Setup
	pk, _, err := plonk.Setup(proofSystem.Ccs, proofSystem.Srs)
	if err != nil {
		log.Fatal("plonk.Setup", err)
	}

	// plonk: generate proof from witness
	proof, err := plonk.Prove(proofSystem.Ccs, pk, w)
	if err != nil {
		log.Fatal("prove computation failed...", err)
	}
	// call contract verify function
	var (
		input [5]*big.Int
	)

	// public witness, the hash of the secret is on chain
	input[0] = new(big.Int).SetInt64(int64(threshold))
	input[1], _ = new(big.Int).SetString(digest, 10)
	input[2] = merkleInt
	input[3] = new(big.Int).SetBytes(userSigner.PublicKey)
	input[4] = nonceMod

	p := proof.(*plonk_bn254.Proof)
	serializedProof := p.MarshalSolidity()
	// call the contract
	log.Println("on-chain verifying proof with nonce:", nonceMod.String())

	auth = userSigner.BindTxOpts()
	tx, err := verifierContract.Verify(auth, serializedProof[:], input[:])
	if err != nil {
		log.Fatal("transaction failed ", err)
		return
	}

	// get the transaction receipt
	receipt, err := userSigner.WaitForReceipt(tx.Hash())
	if err != nil {
		log.Fatal("Couldn't get transaction receipt", err)
		return
	}

	// check if the transaction was successful
	if receipt.Status != 1 {
		log.Fatal("transaction failed", receipt.Status)
		return
	}

	_, err = proof.WriteTo(&buf)
	if err != nil {
		return
	}
	// start timer
	startTime := time.Now()

	receipt, err = client.SetPrivateClaim(manClient, userSigner, "merkleCombined", buf.String(),
		"age > 22 inclusion", circuitMeta.IpfsURI, receipt.Logs[0].Topics[0].String())
	elapsedTime := time.Since(startTime)
	fmt.Printf("SetPrivateClaim took: %s\n", elapsedTime)
	if err != nil {
		return
	}
	waitForReceipt, err := userSigner.WaitForReceipt(receipt.TxHash)
	if err != nil {
		return
	}
	calculateGasUsage("SetPrivateClaim", waitForReceipt)

	// read private claim on chain
	startTime = time.Now()
	claim, err := client.GetPrivateClaim(manClient, "merkleCombined")
	elapsedTime = time.Since(startTime)
	fmt.Printf("GetPrivateClaim took: %s\n", elapsedTime)
	fmt.Printf("Private claim: id:%d statement:%s eventHash: %s\n", claim.ClaimMeta.Id,
		claim.ClaimMeta.Statement,
		claim.EventHash)

}
func SimulateAttestation(claimOwner *client.Signer, attestor *client.Signer, reader *client.TxReader,
	ipfsConn string, registryAddr string, userIdentityAddr string) {
	fmt.Println("Simulation type: attestation")
	fmt.Println("Pre-required simulations: deploy,mimcDeployment,mimcClaim")

	// register circuit metadata on registry
	regClient := client.NewRegistryClient(registryAddr, *reader)
	manClient := client.NewManagerClient(userIdentityAddr, *reader, ipfsConn)

	//check clients are valid
	if regClient.Registry == nil || manClient.Manager == nil {
		log.Fatal("Check contract addresses")
		return
	}

	startTime := time.Now()
	// attestor read claim from user contract
	privateClaim, err := client.GetPrivateClaim(manClient, "agePriv")
	if err != nil {
		log.Fatal("Could not get private claim", err)
		return
	}
	// sign the claim with attestor {t, claim={commitment/public_value}, statement}
	jsonMeta, err := json.Marshal(privateClaim.ClaimMeta)
	if err != nil {
		return
	}

	claimSig, err := attestor.Sign(jsonMeta) // we assume signature is sent to user in a secure channel
	if err != nil {
		log.Fatal("Could not sign claim", err)
		return
	}
	expires := new(big.Int).SetUint64(1000000000000000000)
	id := new(big.Int).SetUint64(uint64(privateClaim.ClaimMeta.Id))
	// user gets signature related with the claim and sets attestation in usersigner manager contract
	receipt, err := client.SetAttestation(manClient, claimOwner, "agePriv", common.BytesToAddress(attestor.PublicKey),
		claimSig, *expires, *id)

	if err != nil {
		log.Fatal("Could not set attestation", err)
		return
	}
	elapsedTime := time.Since(startTime)
	fmt.Printf("Attestation set took: %s\n", elapsedTime)
	calculateGasUsage("Set attestation", receipt)

	// attestor reads attestation from user contract
	attestation, err := client.GetAttestation(manClient, "agePriv")
	if err != nil {
		log.Fatal("Could not get attestation", err)
		return
	}
	attestJson, _ := json.Marshal(attestation)
	fmt.Printf(
		"Attestation: %s\n",
		attestJson,
	)

}
func SimulateRevocation(claimOwner *client.Signer, attestor *client.Signer, reader *client.TxReader, ipfsConn string,
	userIdentityAddr string, attestorIdentityAddr string) {
	fmt.Println("Simulation type: revocation")
	fmt.Println("Pre-required simulations: deploy,mimcDeployment,mimcClaim, attestation")

	userIdentityClient := client.NewManagerClient(userIdentityAddr, *reader, ipfsConn)
	attestorIdentityClient := client.NewManagerClient(attestorIdentityAddr, *reader, ipfsConn)

	// attestor reads attestation from user contract
	attestation, err := client.GetAttestation(userIdentityClient, "agePriv")
	if err != nil {
		log.Fatal("Could not get attestation", err)
		return
	}
	startTime := time.Now()
	id := new(big.Int).SetInt64(int64(attestation.Id))
	receipt, err := client.Revoke(attestorIdentityClient, attestor, "ageAttestation",
		common.BytesToAddress(claimOwner.PublicKey), id, "Invalid age")
	receipt, err = attestor.WaitForReceipt(receipt.TxHash)

	if err != nil {
		log.Fatal("Could not revoke claim", err)
		return
	}
	elapsedTime := time.Since(startTime)
	fmt.Printf("Revocation took: %s\n", elapsedTime)
	calculateGasUsage("Revocation", receipt)

	//4. User should be able to see revoked attestations
	startTime = time.Now()
	revocation, err := client.GetRevocation(attestorIdentityClient, "ageAttestation")
	if err != nil {
		log.Fatal("Could not get attestation", err)
		return
	}
	elapsedTime = time.Since(startTime)
	fmt.Printf("Revocation from contract: %s %s %d %d\n", revocation.AttestedTo, revocation.Status,
		revocation.AttestationId, revocation.Timestamp)
	fmt.Printf("Get revocation took: %s\n", elapsedTime)

}

func GetCircuitInfoFromRegistry(regClient *client.RegistryClient, circuitName string) {
	circuitInfo, err := client.GetCircuit(regClient, circuitName)
	if err != nil {
		log.Fatal("Could not get circuit info", err)
		return
	}
	fmt.Printf("CircuitMeta info: %s %s %s %s\n", circuitInfo.Address, circuitInfo.DeploymentType, circuitInfo.IpfsURI)
}

func weiToEther(wei uint64) *big.Float {
	weiInEther := new(big.Float).SetUint64(wei)
	ether := new(big.Float).Quo(weiInEther, big.NewFloat(1e18))
	return ether
}

func calculateGasUsage(operation string, receipt *types.Receipt) {
	fmt.Printf("%s Transaction Cost (Wei): %d \n", operation, receipt.GasUsed)
	fmt.Printf("%s Transaction Cost (Ether): %s\n", operation, weiToEther(receipt.GasUsed).String())
	total_cost := new(big.Int)
	total_gas := new(big.Int).SetUint64(receipt.GasUsed)
	total_cost.Mul(total_gas, gasPrice)
}

func GenerateDummyClaim() []byte {
	claimDict := make(map[string]string)
	claimDict["age"] = strconv.Itoa(rand.Int())
	claimDict["timestamp"] = strconv.Itoa(rand.Int())
	claimDict["revoked"] = "false"
	claimDict["subject"] = "0x2e144aF3Bde9B518C7C65FBE170c07c888f1fF1a"
	jsonStr, _ := json.Marshal(claimDict)
	return jsonStr
}
