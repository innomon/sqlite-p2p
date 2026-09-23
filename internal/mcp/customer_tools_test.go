package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCustomerTools_Lifecycle(t *testing.T) {
	repo, keys, tracker, cleanup := setupTestDB(t)
	defer cleanup()

	handler := NewCustomerToolHandler(repo, keys, tracker)
	ctx := context.Background()

	phone := "+1-555-123-4567"
	meta := `{"tier":"gold","schemas":[{"$ID":"https://crm.local/customer-v1.json"}]}`
	payload := "Confidential Health and Financial Profile"

	// 1. CustomerPut
	putInput := CustomerPutInput{
		PhoneOrKey: phone,
		Metadata:   meta,
		Payload:    payload,
	}
	res, putOut, err := handler.CustomerPut(ctx, nil, putInput)
	if err != nil {
		t.Fatalf("CustomerPut failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("CustomerPut returned tool error: %v", res.Content)
	}
	if putOut.Key == "" {
		t.Fatal("expected non-empty key")
	}

	// 2. CustomerGet
	getInput := CustomerGetInput{PhoneOrKey: phone}
	res, getOut, err := handler.CustomerGet(ctx, nil, getInput)
	if err != nil {
		t.Fatalf("CustomerGet failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("CustomerGet returned tool error")
	}
	if getOut.Payload != payload {
		t.Errorf("expected decrypted payload '%s', got '%s'", payload, getOut.Payload)
	}

	// 3. CustomerList
	listInput := CustomerListInput{Limit: 10}
	res, listOut, err := handler.CustomerList(ctx, nil, listInput)
	if err != nil {
		t.Fatalf("CustomerList failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("CustomerList returned tool error")
	}
	if listOut.Count != 1 {
		t.Fatalf("expected count 1, got %d", listOut.Count)
	}

	// 4. CustomerShred (crypto-shredding)
	shredInput := CustomerShredInput{PhoneOrKey: phone}
	res, shredOut, err := handler.CustomerShred(ctx, nil, shredInput)
	if err != nil {
		t.Fatalf("CustomerShred failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("CustomerShred returned tool error")
	}
	if !shredOut.Success {
		t.Error("expected shredding success")
	}

	// After shredding, Get should fail decryption or indicate crypto-shredded
	res, _, err = handler.CustomerGet(ctx, nil, getInput)
	if err != nil {
		t.Fatalf("unexpected error on get after shred: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected error getting crypto-shredded customer payload")
	}

	// 5. CustomerDelete
	delInput := CustomerDeleteInput{PhoneOrKey: phone}
	res, delOut, err := handler.CustomerDelete(ctx, nil, delInput)
	if err != nil {
		t.Fatalf("CustomerDelete failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("CustomerDelete returned tool error")
	}
	if !delOut.Success {
		t.Error("expected delete success")
	}
}

func TestCustomerTools_Validation(t *testing.T) {
	repo, keys, tracker, cleanup := setupTestDB(t)
	defer cleanup()

	handler := NewCustomerToolHandler(repo, keys, tracker)
	ctx := context.Background()

	// Empty phone/key
	res, _, _ := handler.CustomerPut(ctx, nil, CustomerPutInput{
		PhoneOrKey: "",
		Metadata:   `{"test":true}`,
	})
	if res == nil || !res.IsError {
		t.Fatal("expected error on empty phone_or_key")
	}

	// Invalid metadata JSON
	res, _, _ = handler.CustomerPut(ctx, nil, CustomerPutInput{
		PhoneOrKey: "+1-555-000-0000",
		Metadata:   `{invalid`,
	})
	if res == nil || !res.IsError {
		t.Fatal("expected error on invalid metadata JSON")
	}

	// Get non-existent
	res, _, _ = handler.CustomerGet(ctx, nil, CustomerGetInput{
		PhoneOrKey: "+1-999-999-9999",
	})
	if res == nil || !res.IsError {
		t.Fatal("expected error on getting non-existent customer")
	}
	if text, ok := res.Content[0].(*sdk.TextContent); ok {
		if !strings.Contains(text.Text, "failed") {
			t.Errorf("expected error message to contain 'failed', got: %s", text.Text)
		}
	}
}
