package flextime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScanFromDatabasePatterns(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		src    interface{}
		want   time.Time
	}{
		{
			name:   "PostgreSQL timestamptz via lib/pq",
			driver: "lib/pq",
			src:    time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
			want:   time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:   "MySQL datetime via go-sql-driver",
			driver: "go-sql-driver/mysql",
			src:    []byte("2024-03-15 10:30:00"),
			want:   time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:   "MySQL date via go-sql-driver",
			driver: "go-sql-driver/mysql",
			src:    []byte("2024-03-15"),
			want:   time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "SQLite text ISO",
			driver: "mattn/go-sqlite3",
			src:    "2024-03-15 10:30:00",
			want:   time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:   "SQLite Unix timestamp",
			driver: "mattn/go-sqlite3",
			src:    int64(1710505800),
			want:   time.Unix(1710505800, 0),
		},
		{
			name:   "NULL value",
			driver: "any",
			src:    nil,
			want:   time.Time{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.Scan(tt.src)
			if err != nil {
				t.Fatalf("Scan(%T) error: %v", tt.src, err)
			}
			if tt.src == nil {
				if ft.Valid() {
					t.Error("expected invalid for NULL")
				}
				return
			}
			if !ft.Valid() {
				t.Error("expected valid")
			}
			if !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestJSONStructIntegration(t *testing.T) {
	jsonData := `{
		"created": "2024-03-15T10:30:00Z",
		"modified": "03/15/2024",
		"epoch": 1710505800,
		"deleted": null
	}`

	type APIResponse struct {
		Created  FlexTime `json:"created"`
		Modified FlexTime `json:"modified"`
		Epoch    FlexTime `json:"epoch"`
		Deleted  FlexTime `json:"deleted"`
	}

	var resp APIResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !resp.Created.Valid() {
		t.Error("Created should be valid")
	}
	if !resp.Modified.Valid() {
		t.Error("Modified should be valid")
	}
	if !resp.Epoch.Valid() {
		t.Error("Epoch should be valid")
	}
	if resp.Deleted.Valid() {
		t.Error("Deleted should be invalid (null)")
	}

	// All three valid times should represent the same day.
	y1, m1, d1 := resp.Created.Time().Date()
	y2, m2, d2 := resp.Modified.Time().Date()
	if y1 != y2 || m1 != m2 || d1 != d2 {
		t.Errorf("Created and Modified dates differ: %v vs %v",
			resp.Created.Time(), resp.Modified.Time())
	}
}

func TestScanValueRoundTrip(t *testing.T) {
	original := New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))

	v, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	var restored FlexTime
	if err := restored.Scan(v); err != nil {
		t.Fatalf("Scan(Value()) error: %v", err)
	}

	if !restored.Time().Equal(original.Time()) {
		t.Errorf("round-trip failed: got %v, want %v", restored.Time(), original.Time())
	}
}

func TestScanValueNullRoundTrip(t *testing.T) {
	var original FlexTime

	v, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v != nil {
		t.Fatalf("Value() = %v, want nil", v)
	}

	var restored FlexTime
	if err := restored.Scan(v); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if restored.Valid() {
		t.Error("expected invalid after null round-trip")
	}
}
