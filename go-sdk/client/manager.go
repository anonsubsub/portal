package client

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go-sdk/contracts/IdentityManager"
	"log"
	"math/big"
	"time"
)

type ManagerClient struct {
	Manager    *IdentityManager.IdentityManager
	IpfsClient *IpfsClient
}

type ClaimMeta struct {
	Value     string  `json:"value"`
	Statement string  `json:"statement"`
	Timestamp big.Int `json:"timestamp"`
	Id        int     `json:"id"`
	Version   int     `json:"version"`
}
type PublicClaim struct {
	ClaimMeta ClaimMeta
}

func NewPublicClaim(claimMeta ClaimMeta) *PublicClaim {
	return &PublicClaim{ClaimMeta: claimMeta}
}

type PrivateClaim struct {
	ClaimMeta ClaimMeta
	IpfsURI   string
	EventHash string
}

func NewPrivateClaim(claimMeta ClaimMeta, ipfsURI string, eventHash string) *PrivateClaim {
	return &PrivateClaim{ClaimMeta: claimMeta, IpfsURI: ipfsURI, EventHash: eventHash}
}

type Attestation struct {
	Signature []byte
	Attestor  string
	Timestamp big.Int
	ExpiresAt big.Int
	Id        int
	ClaimId   int
}

type Revocation struct {
	AttestedTo    common.Address
	AttestationId *big.Int
	Status        string
	Timestamp     *big.Int
}

func NewAttestation(signature []byte, attestor string, timestamp big.Int, expiresAt big.Int, id int, claimId int) *Attestation {
	return &Attestation{Signature: signature, Attestor: attestor, Timestamp: timestamp, ExpiresAt: expiresAt, Id: id, ClaimId: claimId}
}

func NewManagerClient(contractAddr string, reader TxReader, ipfsConn string) *ManagerClient {
	contract := common.HexToAddress(contractAddr)
	instance, err := IdentityManager.NewIdentityManager(contract, reader.Client)
	if err != nil {
		log.Fatal(err)
	}

	ipfsClient := NewIpfsClient(ipfsConn)
	return &ManagerClient{
		Manager:    instance,
		IpfsClient: ipfsClient,
	}
}

func SetPublicClaim(client *ManagerClient, signer *Signer, key string, value string, statement string) (*types.Receipt, error) {
	auth := signer.BindTxOpts()
	// convert time now to unix timestamp

	timestamp := time.Now().Unix()
	timeInBig := new(big.Int).SetInt64(timestamp)
	id := timeInBig
	tx, err := client.Manager.SetPublicClaim(auth, key, value, statement, timeInBig, id)
	if err != nil {
		return nil, err
	}

	receipt, err := signer.WaitForReceipt(tx.Hash())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

func GetPublicClaim(client *ManagerClient, key string) (*PublicClaim, error) {
	claim, err := client.Manager.PublicClaims(&bind.CallOpts{}, key)
	if err != nil {
		return nil, err
	}
	return NewPublicClaim(ClaimMeta{
		Value:     claim.Value,
		Statement: claim.Statement,
		Timestamp: *claim.Timestamp,
		Id:        int(claim.Id.Int64()),
	}), nil
}

func SetPrivateClaim(client *ManagerClient, signer *Signer, key string, value string, statement string, ipfsURI string,
	eventHash string) (*types.Receipt, error) {
	auth := signer.BindTxOpts()
	timestamp := time.Now().Unix()
	timeInBig := new(big.Int).SetInt64(timestamp)
	id := timeInBig
	tx, err := client.Manager.SetPrivateClaim(auth, key, value, statement, ipfsURI, eventHash, timeInBig, id)
	if err != nil {
		return nil, err
	}

	receipt, err := signer.WaitForReceipt(tx.Hash())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}
func GetPrivateClaim(client *ManagerClient, key string) (*PrivateClaim, error) {
	claim, err := client.Manager.PrivateClaims(&bind.CallOpts{}, key)
	if err != nil {
		return nil, err
	}
	return NewPrivateClaim(ClaimMeta{
		Value:     claim.Value,
		Statement: claim.Statement,
		Timestamp: *claim.Timestamp,
		Id:        int(claim.Id.Int64()),
		Version:   int(claim.Version.Int64()),
	}, claim.IpfsCircuitMetadata, claim.EventHash), nil
}

//func DeleteClaim(client *ManagerClient, signer *Signer, key string) (*types.Receipt, error) {
//	auth := signer.BindTxOpts()
//
//	tx, err := client.Manager.DeleteClaim(auth, key)
//	if err != nil {
//		return nil, err
//	}
//
//	receipt, err := signer.WaitForReceipt( tx.Hash())
//	if err != nil {
//		return nil, err
//	}
//
//	return receipt, nil
//}

func GetClaimURI(client *ManagerClient, claimHash string) (string, error) {
	val, err := client.Manager.IpfsClaims(nil, claimHash)
	if err != nil {
		return "", err
	}

	return val, nil
}

func setClaimURI(client *ManagerClient, signer *Signer, claimHash string, uri string) (*types.Receipt, error) {
	auth := signer.BindTxOpts()

	tx, err := client.Manager.SetClaimURI(auth, claimHash, uri)
	if err != nil {
		return nil, err
	}

	receipt, err := signer.WaitForReceipt(tx.Hash())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

func PublishClaim(client *ManagerClient, signer *Signer, key string, value []byte) (*types.Receipt, error) {
	//write claim first on ipfs
	resp, err := client.IpfsClient.AddAndPublish(value)
	if err != nil {
		return nil, err
	}
	ipnsPath := resp["Name"]

	return setClaimURI(client, signer, key, ipnsPath)
}

func SetAttestation(client *ManagerClient, signer *Signer, key string, attestor common.Address,
	signature []byte, expires big.Int, claimId big.Int) (*types.Receipt, error) {
	auth := signer.BindTxOpts()
	tx, err := client.Manager.SetAttestation(auth, key, attestor, &expires, signature, &claimId, &claimId)
	if err != nil {
		return nil, err
	}

	receipt, err := signer.WaitForReceipt(tx.Hash())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

func GetAttestation(client *ManagerClient, key string) (*Attestation, error) {
	attestation, err := client.Manager.Attestations(&bind.CallOpts{}, key)
	if err != nil {
		return nil, err
	}

	return NewAttestation(
		attestation.Signature,
		attestation.Attestor.Hex(),
		*attestation.Timestamp,
		*attestation.Expires,
		int(attestation.Id.Int64()),
		int(attestation.ClaimId.Int64())), nil
}

func Revoke(client *ManagerClient, signer *Signer, key string, attestedTo common.Address,
	id *big.Int, reason string) (*types.Receipt, error) {
	auth := signer.BindTxOpts()
	tx, err := client.Manager.RevokeAttestation(auth, key, reason, attestedTo, id)
	if err != nil {
		return nil, err
	}

	receipt, err := signer.WaitForReceipt(tx.Hash())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

func GetRevocation(client *ManagerClient, key string) (Revocation, error) {
	revocation, err := client.Manager.Revocations(&bind.CallOpts{}, key)
	if err != nil {
		return Revocation{}, err
	}

	return revocation, nil
}
