// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package testproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
)

func TestCosmosDBTables(t *testing.T) {
	//=====================================================================//
	// Test proxy prologue. The following code is necessary to configure   //
	// the test proxy, as well as to start the record or playback process. //
	//=====================================================================//
	// Load environment variables from the local .env file
	root := GetCurrentDirectory()
	err := Load(filepath.Join(root, ".env"))
	if err != nil {
		log.Fatal(err)
	}

	userproxy, err := strconv.ParseBool(os.Getenv("USE_PROXY"))
	if err != nil {
		log.Fatal(err)
	}

	tpv := &TestProxy{}
	if userproxy == true {
		tpv.Mode = os.Getenv("PROXY_MODE")
		tpv.Host = os.Getenv("PROXY_HOST")
		port, err := strconv.Atoi(os.Getenv("PROXY_PORT"))
		if err != nil {
			t.Fatal(err)
		}
		tpv.Port = port
		tpv.RecordingPath = root

		if err = StartTestProxy(t, tpv); err != nil {
			t.Fatal(err)
		}

		defer func() {
			err = StopTestProxy(t, tpv)
			if err != nil {
				t.Fatal(err)
			}
		}()
	}

	options, err := GetClientOption(tpv, &client)
	if err != nil {
		log.Fatal(err)
	}

	//=========================================================================================//
	// End of test proxy prologue. Original test code starts here. Everything after this point //
	// represents an app interacting with the Azure Table Storage service.                     //
	//=========================================================================================//
	tableServiceClient, err := aztables.NewServiceClientFromConnectionString(os.Getenv("COSMOS_CONNECTION_STRING"), &aztables.ClientOptions{
		ClientOptions: options.ClientOptions,
	})
	if err != nil {
		t.Fatal(err)
	}

	// New instance of TableClient class referencing the server-side table
	tableClient := tableServiceClient.NewClient("gocosmosZ")
	_, err = tableClient.CreateTable(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create new item using composite key constructor
	rowKey := "68719518388"
	partitionKey := "gear-surf-surfboards"
	prod1 := Product{
		RowKey:       rowKey,
		PartitionKey: partitionKey,
		Name:         "Ocean Surfboard",
		Quantity:     8,
		Sale:         true,
	}

	// Add new item to server-side table
	entity, err := json.Marshal(prod1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tableClient.AddEntity(context.Background(), entity, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Read a single item from container
	resp, err := tableClient.GetEntity(context.Background(), partitionKey, rowKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	product := Product{}
	err = json.Unmarshal(resp.Value, &product)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Single product:")
	fmt.Println(product.Name)

	// Read multiple items from container
	prod2 := Product{
		RowKey:       "68719518390",
		PartitionKey: "gear-surf-surfboards",
		Name:         "Sand Surfboard",
		Quantity:     5,
		Sale:         false,
	}

	entity, err = json.Marshal(prod2)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tableClient.AddEntity(context.Background(), entity, nil)
	if err != nil {
		t.Fatal(err)
	}

	pager := tableClient.NewListEntitiesPager(&aztables.ListEntitiesOptions{})
	for pager.More() {
		result, err := pager.NextPage(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range result.Entities {
			product := Product{}
			err = json.Unmarshal(e, &product)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(product.Name)
		}
	}

	_, err = tableClient.Delete(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

}

type Product struct {
	RowKey       string
	PartitionKey string
	Name         string
	Quantity     int
	Sale         bool
}
