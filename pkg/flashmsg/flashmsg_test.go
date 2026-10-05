package flashmsg

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestBag(t *testing.T) {
	for _, tt := range []struct {
		name string
		bag  *Bag
	}{
		{name: "constructor", bag: New()},
		{name: "zero value", bag: &Bag{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bag := tt.bag
			bag.Add(Success, "First")
			bag.Add(Error, "Invalid input")
			bag.Add(Success, "Second")
			bag.Add("custom", "Application message")

			if !slices.Equal(bag.Types(), []string{"custom", Error, Success}) {
				t.Fatalf("Types() = %v", bag.Types())
			}

			peek := bag.Peek(Success)
			peek[0] = "changed"
			all := bag.PeekAll()
			all[Success][1] = "changed"
			delete(all, Error)

			if got := bag.Get(Success); !slices.Equal(got, []string{"First", "Second"}) {
				t.Fatalf("Get(success) = %v", got)
			}

			if len(bag.Get(Success)) != 0 || len(bag.Get("missing")) != 0 {
				t.Fatal("messages were consumed more than once")
			}

			want := map[string][]string{Error: {"Invalid input"}, "custom": {"Application message"}}

			if got := bag.All(); !reflect.DeepEqual(got, want) || len(bag.All()) != 0 {
				t.Fatalf("All() = %v, want %v", got, want)
			}

			bag.Add(Info, "New message")

			if !slices.Equal(bag.Get(Info), []string{"New message"}) {
				t.Fatal("bag could not be reused after consuming it")
			}
		})
	}
}

func TestSerialization(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
		want map[string][]string
	}{
		{name: "empty", data: `{}`, want: map[string][]string{}},
		{name: "null", data: `null`, want: map[string][]string{}},
		{name: "typed messages", data: `{"success":["Saved","Updated"],"error":["Failed"],"custom":["Pārskatītāji & <script>"]}`, want: map[string][]string{Success: {"Saved", "Updated"}, Error: {"Failed"}, "custom": {"Pārskatītāji & <script>"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bag := New()

			if err := json.Unmarshal([]byte(tt.data), bag); err != nil {
				t.Fatal(err)
			}

			data, err := json.Marshal(bag)

			if err != nil {
				t.Fatal(err)
			}

			var roundtrip Bag

			if err := json.Unmarshal(data, &roundtrip); err != nil {
				t.Fatal(err)
			}

			if got := roundtrip.All(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("roundtrip = %v, want %v", got, tt.want)
			}
		})
	}

	for _, data := range []string{`{`, `[]`, `{"error":"wrong type"}`, `{"error":[1]}`} {
		t.Run("invalid "+data, func(t *testing.T) {
			bag := New()
			bag.Add(Success, "Keep this")

			if err := json.Unmarshal([]byte(data), bag); err == nil || !slices.Equal(bag.Get(Success), []string{"Keep this"}) {
				t.Fatalf("failed decode changed the bag: %v", err)
			}
		})
	}
}

func TestContext(t *testing.T) {
	bag := New()
	bag.Add(Success, "Saved")
	ctx := WithBag(context.Background(), bag)

	for _, tt := range []struct {
		name string
		ctx  context.Context
		want []string
	}{
		{name: "attached", ctx: ctx, want: []string{"Saved"}},
		{name: "unrelated request", ctx: context.Background()},
		{name: "nil bag", ctx: WithBag(context.Background(), nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromContext(tt.ctx).Peek(Success); !slices.Equal(got, tt.want) {
				t.Fatalf("context messages = %v, want %v", got, tt.want)
			}
		})
	}

	FromContext(ctx).Get(Success)

	if len(bag.Peek(Success)) != 0 {
		t.Fatal("request context did not share the attached bag")
	}
}
