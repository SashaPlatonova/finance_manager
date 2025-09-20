package memory

import (
	"finance_manager/internal/repository"
)

var (
	_ repository.TransactionRepository = (*TransactionRepository)(nil)
	_ repository.AccountRepository     = (*AccountRepository)(nil)
	_ repository.RuleRepository        = (*RuleRepository)(nil)
)
