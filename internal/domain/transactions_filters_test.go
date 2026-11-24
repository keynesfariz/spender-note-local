package domain

import "testing"

func TestTransactions_FilterByCategoryOrTagNamedAll(t *testing.T) {
	gdb := openDomainTestDB(t)
	category, err := CreateCategory(gdb, CreateCategoryInput{Name: "all", Icon: "Tag"})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := CreateTag(gdb, CreateTagInput{Name: "all"})
	if err != nil {
		t.Fatal(err)
	}
	match, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   100,
		OccurredAtUtc: "2026-11-21T08:00:00Z",
		Merchant:      "Matching transaction",
		Category:      &category.Name,
		TagIDs:        []string{tag.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateManualTransaction(gdb, CreateManualTransactionInput{
		AmountMinor:   200,
		OccurredAtUtc: "2026-11-21T09:00:00Z",
		Merchant:      "Other transaction",
	}); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []ListTransactionsParams{{Category: "all"}, {Tag: "all"}} {
		result, err := ListTransactions(gdb, filter)
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalCount != 1 || len(result.Items) != 1 || result.Items[0].ID != match.ID {
			t.Fatalf("filter %+v should return only %s, got %+v", filter, match.ID, result.Items)
		}
	}
}
