package models

import "gorm.io/gorm"

func ensureResellerIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	statements := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_reseller_domains_active_domain ON reseller_domains(domain) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_reseller_site_configs_active_reseller ON reseller_site_configs(reseller_id) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_reseller_product_settings_active_scope ON reseller_product_settings(reseller_id, product_id, sku_id) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_reseller_balance_accounts_active_currency ON reseller_balance_accounts(reseller_id, currency) WHERE deleted_at IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_reseller_related_accounts_active_user ON reseller_related_accounts(reseller_id, user_id) WHERE deleted_at IS NULL",
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
