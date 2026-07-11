package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestTemplateImplementsOperations(t *testing.T) {
	var _ Operations = (*Template)(nil)
}

func TestOperationsRejectNilClient(t *testing.T) {
	template := NewTemplate(nil)
	if err := template.HashSet(context.Background(), "key", map[string]string{"field": "value"}); err == nil {
		t.Fatal("expected hash operation to reject nil client")
	}
	if err := template.SortedSetAdd(context.Background(), "key", SortedSetMember{Value: "value", Score: 1}); err == nil {
		t.Fatal("expected sorted set operation to reject nil client")
	}
}

func TestSortedSetMembersMapsValues(t *testing.T) {
	result := sortedSetMembers([]goredis.Z{{Member: "first", Score: 2}})
	if len(result) != 1 || result[0].Value != "first" || result[0].Score != 2 {
		t.Fatalf("unexpected members: %#v", result)
	}
}
