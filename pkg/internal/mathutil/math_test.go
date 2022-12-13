package mathutil

import "testing"

func TestFloorDiv(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"1/1", 1, 1, 1},
		{"1/2", 1, 2, 0},
		{"2/1", 2, 1, 2},
		{"2/2", 2, 2, 1},

		{"-4/3", -4, 3, -2},
		{"-1/-2", -1, -2, 0},
		{"-2/-1", -2, -1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FloorDiv(tt.a, tt.b); got != tt.want {
				t.Errorf("FloorDiv() = %v, want %v", got, tt.want)
			}
		})
	}
}
