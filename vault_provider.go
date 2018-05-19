package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/credentials"
	vault "github.com/hashicorp/vault/api"
)

type VaultProvider struct {
	client *vault.Client
}

func (v *VaultProvider) Retrieve() (credentials.Value, error) {
	if os.Getenv("VAULT_TOKEN") == "" {
		panic("must set VAULT_TOKEN")
	}

	if os.Getenv("VAULT_AWS_SECRETS_ROLE") == "" {
		panic("must set VAULT_AWS_SECRETS_ROLE")
	}

	secret, err := v.client.Logical().Read(fmt.Sprintf("aws/creds/%s", os.Getenv("VAULT_AWS_SECRETS_ROLE")))
	if err != nil {
		panic(err)
	} else {
		fmt.Println("created iam user through vault")
	}

	access_key := secret.Data["access_key"]
	secret_key := secret.Data["secret_key"]

	return credentials.Value{
		AccessKeyID:     access_key.(string),
		SecretAccessKey: secret_key.(string),
	}, nil
}

func (v *VaultProvider) IsExpired() bool {
	return false
}
