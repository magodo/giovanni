package testhelpers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storage/2023-01-01/storageaccounts"
	"github.com/hashicorp/go-azure-sdk/sdk/auth"
	authWrapper "github.com/hashicorp/go-azure-sdk/sdk/auth/autorest"
	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/client/dataplane/storage"
	"github.com/hashicorp/go-azure-sdk/sdk/environments"
)

type Client struct {
	Environment          environments.Environment
	ResourceGroupsClient *resources.GroupsClient
	StorageAccountClient *storageaccounts.StorageAccountsClient
	SubscriptionId       string

	resourceManagerAuth auth.Authorizer
	storageAuth         auth.Authorizer

	// TODO: this can be removed once `resources` and `storage` are updated to use `hashicorp/go-azure-sdk`
	resourceManagerAuthorizer autorest.Authorizer
}

type TestResources struct {
	ResourceGroup      string
	StorageAccountName string
	StorageAccountKey  string
}

func (c Client) BuildTestResources(ctx context.Context, resourceGroup, name string, kind storageaccounts.Kind) (*TestResources, error) {
	return c.buildTestResources(ctx, resourceGroup, name, kind, false, "")
}
func (c Client) BuildTestResourcesWithHns(ctx context.Context, resourceGroup, name string, kind storageaccounts.Kind) (*TestResources, error) {
	return c.buildTestResources(ctx, resourceGroup, name, kind, true, "")
}
func (c Client) BuildTestResourcesWithSku(ctx context.Context, resourceGroup, name string, kind storageaccounts.Kind, sku storageaccounts.SkuName) (*TestResources, error) {
	return c.buildTestResources(ctx, resourceGroup, name, kind, false, sku)
}
func (c Client) buildTestResources(ctx context.Context, resourceGroup, name string, kind storageaccounts.Kind, enableHns bool, sku storageaccounts.SkuName) (*TestResources, error) {
	location := os.Getenv("ARM_TEST_LOCATION")
	_, err := c.ResourceGroupsClient.CreateOrUpdate(ctx, resourceGroup, resources.Group{
		Location: &location,
	})

	if err != nil {
		return nil, fmt.Errorf("error creating Resource Group %q: %s", resourceGroup, err)
	}

	props := storageaccounts.StorageAccountPropertiesCreateParameters{
		AllowBlobPublicAccess: pointer.To(true),
		PublicNetworkAccess:   pointer.To(storageaccounts.PublicNetworkAccessEnabled),
	}
	if kind == storageaccounts.KindBlobStorage {
		props.AccessTier = pointer.To(storageaccounts.AccessTierHot)
	}
	if enableHns {
		props.IsHnsEnabled = &enableHns
	}
	if sku == "" {
		sku = storageaccounts.SkuNameStandardLRS
	}

	payload := storageaccounts.StorageAccountCreateParameters{
		Location: location,
		Sku: storageaccounts.Sku{
			Name: sku,
		},
		Kind:       kind,
		Properties: &props,
	}
	id := commonids.NewStorageAccountID(c.SubscriptionId, resourceGroup, name)
	if err = c.StorageAccountClient.CreateThenPoll(ctx, id, payload); err != nil {
		return nil, fmt.Errorf("error creating Account %q (Resource Group %q): %s", name, resourceGroup, err)
	}

	var options storageaccounts.ListKeysOperationOptions
	keys, err := c.StorageAccountClient.ListKeys(ctx, id, options)
	if err != nil {
		return nil, fmt.Errorf("error listing keys for Storage Account %q (Resource Group %q): %s", name, resourceGroup, err)
	}

	// sure we could poll to get around the inconsistency, but where's the fun in that
	time.Sleep(5 * time.Second)

	accountKeys := *keys.Model.Keys
	return &TestResources{
		ResourceGroup:      resourceGroup,
		StorageAccountName: name,
		StorageAccountKey:  *(accountKeys[0]).Value,
	}, nil
}

func (c Client) DestroyTestResources(ctx context.Context, resourceGroup, name string) error {
	accountId := commonids.NewStorageAccountID(c.SubscriptionId, resourceGroup, name)
	_, err := c.StorageAccountClient.Delete(ctx, accountId)
	if err != nil {
		return fmt.Errorf("error deleting Account %q (Resource Group %q): %s", name, resourceGroup, err)
	}

	future, err := c.ResourceGroupsClient.Delete(ctx, resourceGroup)
	if err != nil {
		return fmt.Errorf("error deleting Resource Group %q: %s", resourceGroup, err)
	}

	err = future.WaitForCompletionRef(ctx, c.ResourceGroupsClient.Client)
	if err != nil {
		return fmt.Errorf("error waiting for deletion of Resource Group %q: %s", resourceGroup, err)
	}

	return nil
}

func Build(ctx context.Context, t *testing.T) (*Client, error) {
	if os.Getenv("ACCTEST") == "" {
		t.Skip("Skipping as `ACCTEST` hasn't been set")
	}

	environmentName := os.Getenv("ARM_ENVIRONMENT")
	env, err := environments.FromName(environmentName)
	if err != nil {
		return nil, fmt.Errorf("determining environment %q: %+v", environmentName, err)
	}
	if env == nil {
		return nil, fmt.Errorf("environment was nil: %s", err)
	}

	authConfig := auth.Credentials{
		Environment:  *env,
		ClientID:     os.Getenv("ARM_CLIENT_ID"),
		TenantID:     os.Getenv("ARM_TENANT_ID"),
		ClientSecret: os.Getenv("ARM_CLIENT_SECRET"),

		EnableAuthenticatingUsingClientCertificate: true,
		EnableAuthenticatingUsingClientSecret:      true,
		EnableAuthenticatingUsingAzureCLI:          false,
		EnableAuthenticatingUsingManagedIdentity:   false,
		EnableAuthenticationUsingOIDC:              false,
		EnableAuthenticationUsingGitHubOIDC:        false,
	}

	resourceManagerAuth, err := auth.NewAuthorizerFromCredentials(ctx, authConfig, authConfig.Environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("unable to build authorizer for Resource Manager API: %+v", err)
	}

	storageAuthorizer, err := auth.NewAuthorizerFromCredentials(ctx, authConfig, authConfig.Environment.Storage)
	if err != nil {
		return nil, fmt.Errorf("unable to build authorizer for Storage API: %+v", err)
	}

	client := Client{
		Environment:    *env,
		SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),

		// internal
		resourceManagerAuth: resourceManagerAuth,
		storageAuth:         storageAuthorizer,

		// Legacy / to be removed
		resourceManagerAuthorizer: authWrapper.AutorestAuthorizer(resourceManagerAuth),
	}

	resourceManagerEndpoint, ok := authConfig.Environment.ResourceManager.Endpoint()
	if !ok {
		return nil, fmt.Errorf("Resource Manager Endpoint is not configured for this environment")
	}

	resourceGroupsClient := resources.NewGroupsClientWithBaseURI(*resourceManagerEndpoint, client.SubscriptionId)
	resourceGroupsClient.Authorizer = client.resourceManagerAuthorizer
	client.ResourceGroupsClient = &resourceGroupsClient

	storageClient, err := storageaccounts.NewStorageAccountsClientWithBaseURI(env.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building client for Storage Accounts: %+v", err)
	}
	storageClient.Client.Authorizer = client.resourceManagerAuth
	client.StorageAccountClient = storageClient

	return &client, nil
}

func (c Client) Configure(client *client.Client, authorizer auth.Authorizer) {
	client.Authorizer = authorizer
	// TODO: add logging
}

func (c Client) PrepareWithResourceManagerAuth(input *storage.BaseClient) {
	input.WithAuthorizer(c.storageAuth)
}

func (c Client) PrepareWithSharedKeyAuth(input *storage.BaseClient, data *TestResources, keyType auth.SharedKeyType) error {
	auth, err := auth.NewSharedKeyAuthorizer(data.StorageAccountName, data.StorageAccountKey, keyType)
	if err != nil {
		return fmt.Errorf("building SharedKey authorizer: %+v", err)
	}
	input.WithAuthorizer(auth)
	return nil
}
