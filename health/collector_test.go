package health

import "testing"

func TestCollectAllPreservesCollectorOrderAndSkipsNil(t *testing.T) {
	first := CollectorFunc(func() []Component { return []Component{{ID: "first"}} })
	second := CollectorFunc(func() []Component { return []Component{{ID: "second"}, {ID: "third"}} })
	components := CollectAll(first, nil, second)
	if len(components) != 3 || components[0].ID != "first" || components[2].ID != "third" {
		t.Fatalf("components = %+v", components)
	}
}
