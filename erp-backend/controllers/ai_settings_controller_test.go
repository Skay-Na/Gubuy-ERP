package controllers

import (
	"strings"
	"testing"
)

func TestAIKeyEncryptionAndRotation(t *testing.T) {
	t.Setenv("AI_CONFIG_SECRET", "test-secret-with-at-least-24-characters")
	const original = "sk-private-provider-key"
	encrypted, err := encryptAIKey(original)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, original) {
		t.Fatal("ciphertext contains original key")
	}
	plain, err := decryptAIKey(encrypted)
	if err != nil || plain != original {
		t.Fatalf("decrypt result = %q, %v", plain, err)
	}
	t.Setenv("AI_CONFIG_SECRET", "another-secret-with-at-least-24-characters")
	if _, err := decryptAIKey(encrypted); err == nil {
		t.Fatal("rotated secret unexpectedly decrypted old key")
	}
}

func TestAIProviderEndpointsAreFixed(t *testing.T) {
	for _, provider := range []string{"deepseek", "minimax", "mimo", "qwen"} {
		if aiProviderURL(provider, "cn") == "" {
			t.Fatalf("missing CN endpoint for %s", provider)
		}
	}
	if aiProviderURL("qwen", "cn") == aiProviderURL("qwen", "global") {
		t.Fatal("Qwen region did not change endpoint")
	}
	if aiProviderCatalog["minimax"].JSONMode {
		t.Fatal("MiniMax should use prompt-only JSON parsing")
	}
}
