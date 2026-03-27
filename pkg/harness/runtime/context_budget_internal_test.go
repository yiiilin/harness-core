package runtime

import "testing"

func TestNormalizeLoopBudgetsDoesNotTreatOtherLegacyFieldsAsExplicitZeroRetries(t *testing.T) {
	budgets := normalizeLoopBudgets(LoopBudgets{
		MaxTotalRuntimeMS: 60000,
	}, nil, nil)

	if budgets.MaxTotalRuntimeMS != 60000 {
		t.Fatalf("expected configured runtime budget to survive normalization, got %#v", budgets)
	}
	if budgets.MaxRetriesPerStep != DefaultLoopBudgets().MaxRetriesPerStep {
		t.Fatalf("expected retry budget to fall back to defaults when only unrelated legacy fields are set, got %#v", budgets)
	}
}

func TestNormalizeLoopBudgetsTreatsDefaultBudgetCloneWithZeroRetriesAsExplicit(t *testing.T) {
	budgets := DefaultLoopBudgets()
	budgets.MaxRetriesPerStep = 0

	normalized := normalizeLoopBudgets(budgets, nil, nil)
	if normalized.MaxRetriesPerStep != 0 {
		t.Fatalf("expected default budget clone with zero retries to preserve explicit zero, got %#v", normalized)
	}
}

func TestNormalizeLoopBudgetsDoesNotTreatSingleDefaultValuedLegacyFieldAsExplicitZeroRetries(t *testing.T) {
	normalized := normalizeLoopBudgets(LoopBudgets{
		MaxSteps: DefaultLoopBudgets().MaxSteps,
	}, nil, nil)

	if normalized.MaxRetriesPerStep != DefaultLoopBudgets().MaxRetriesPerStep {
		t.Fatalf("expected single default-valued legacy field not to imply explicit zero retries, got %#v", normalized)
	}
}
