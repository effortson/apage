package config

import "testing"

// strongCfg is a production config with all secrets set to non-default values.
func strongCfg() *Config {
	return &Config{
		Environment:           "production",
		SessionSecret:         "a-strong-session-secret",
		JWTSigningSecret:      "a-strong-jwt-secret",
		S3AccessKey:           "real-access-key",
		S3SecretKey:           "real-secret-key",
		GatewayInternalSecret: "a-strong-internal-secret",
	}
}

func TestValidate_ProductionRejectsDefaults(t *testing.T) {
	c := strongCfg()
	c.SessionSecret = "dev-session-secret-change-me"
	if err := c.Validate(); err == nil {
		t.Fatal("expected production validation to reject the default session secret")
	}
}

func TestValidate_ProductionRequiresGatewaySecret(t *testing.T) {
	c := strongCfg()
	c.GatewayInternalSecret = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected production validation to require GATEWAY_INTERNAL_SECRET")
	}
}

func TestValidate_ProductionRejectsMinioDefaults(t *testing.T) {
	c := strongCfg()
	c.S3SecretKey = "minioadmin"
	if err := c.Validate(); err == nil {
		t.Fatal("expected production validation to reject default minio credentials")
	}
}

func TestValidate_ProductionAcceptsStrongSecrets(t *testing.T) {
	if err := strongCfg().Validate(); err != nil {
		t.Fatalf("expected strong production config to validate, got %v", err)
	}
}

func TestValidate_ProductionAllowsEmptyS3(t *testing.T) {
	// Tunnel-only production deploys use no cloud storage; empty S3 creds are OK.
	c := strongCfg()
	c.S3AccessKey, c.S3SecretKey = "", ""
	if err := c.Validate(); err != nil {
		t.Fatalf("empty S3 creds must be allowed in production, got %v", err)
	}
}

func TestValidate_DevelopmentSkipsChecks(t *testing.T) {
	c := &Config{Environment: "development"} // all secrets empty/default
	if err := c.Validate(); err != nil {
		t.Fatalf("development must not enforce secret checks, got %v", err)
	}
}

func TestIsProduction(t *testing.T) {
	prod := []string{"production", "prod", "staging"}
	for _, e := range prod {
		if !(&Config{Environment: e}).IsProduction() {
			t.Errorf("%q should be production-like", e)
		}
	}
	nonProd := []string{"", "development", "dev", "test", "local"}
	for _, e := range nonProd {
		if (&Config{Environment: e}).IsProduction() {
			t.Errorf("%q should not be production-like", e)
		}
	}
}
