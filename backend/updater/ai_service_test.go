package updater

import "testing"

func TestAIConfigurationRequiresSecretAndBoundsSpending(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := LoadAIConfig(); err == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("OPENAI_API_KEY", "test-secret")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_VERIFY_MODEL", "")
	t.Setenv("CATALOG_AI_BATCH_SIZE", "")
	t.Setenv("CATALOG_AI_DAILY_REQUEST_LIMIT", "")
	c, err := LoadAIConfig()
	if err != nil || c.Model != "gpt-4.1-mini" || c.VerifyModel != "gpt-4.1" || c.BatchSize != 20 || c.DailyLimit != 40 {
		t.Fatal("invalid defaults", err)
	}
	for _, item := range []struct{ name, value string }{{"OPENAI_MODEL", "https://another-provider.invalid"}, {"CATALOG_AI_DAILY_REQUEST_LIMIT", "100000"}, {"CATALOG_AI_BATCH_SIZE", "0"}} {
		t.Run(item.name, func(t *testing.T) {
			t.Setenv(item.name, item.value)
			if _, err := LoadAIConfig(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestAIConfigAutomaticDeepSeekBackup(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test")
	t.Setenv("DEEPSEEK_MODEL", "")
	t.Setenv("DEEPSEEK_VERIFY_MODEL", "")
	c, err := LoadAIConfig()
	if err != nil || c.DeepSeekModel != "deepseek-flash" || c.DeepSeekVerifyModel != "deepseek-v4-pro" { t.Fatal("invalid backup defaults", err) }
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := LoadAIConfig(); err != nil { t.Fatal("missing primary cannot fall back", err) }
	t.Setenv("DEEPSEEK_MODEL", "https://untrusted.invalid")
	if _, err := LoadAIConfig(); err == nil { t.Fatal("configurable endpoint accepted") }
}
