package proxy

import "testing"

func TestIsProviderError_DetectsPrivacyPolicyMessages(t *testing.T) {
	t.Parallel()

	cases := []string{
		`{"error":{"message":"No endpoints found matching your data policy (Zero data retention). Configure: https://openrouter.ai/settings/privacy"}}`,
		`{"error":{"message":"Model rejected provider.zdr=true"}}`,
		`{"error":{"message":"provider.allow_fallbacks=false is not supported"}}`,
	}

	for _, body := range cases {
		if !IsProviderError([]byte(body)) {
			t.Fatalf("expected provider error for body: %s", body)
		}
	}
}

func TestIsProviderError_IgnoresNonPrivacyErrors(t *testing.T) {
	t.Parallel()

	body := `{"error":{"message":"model not found"}}`
	if IsProviderError([]byte(body)) {
		t.Fatalf("unexpected provider error for body: %s", body)
	}
}
