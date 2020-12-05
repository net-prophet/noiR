package noir

import (
	"strconv"
	"testing"
)

func TestQueueAdd(t *testing.T) {
	for _, driver := range MakeTestDrivers() {
		addTests := []struct {
			add  []string
			want int64
		}{
			{[]string{"a"}, 1},
			{[]string{"a", "b", "c"}, 3},
			{[]string{}, 0},
		}

		for n, tt := range addTests {
			queue := MakeTestQueue(driver, "tests/queue/add/"+strconv.Itoa(n))

			for _, msg := range tt.add {
				if err := queue.Add([]byte(msg)); err != nil {
					t.Errorf("error adding %s to %s: %s", msg, driver, err)

				}
			}
			got, err := queue.Count()
			if err != nil {
				t.Errorf("error getting %s count %s", driver, err)
			}
			if got != tt.want {
				t.Errorf("got %d from %s want %d", got, driver, tt.want)
			}

			if err := queue.Cleanup(); err != nil {
				t.Errorf("error cleaning up %s: %s", driver, err)
			}

		}
	}
}

func TestQueueNext(t *testing.T) {
	for _, driver := range MakeTestDrivers() {
		addTests := []struct {
			add  []string
			want []string
		}{
			{[]string{"a"}, []string{"a"}},
			{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
			{[]string{}, []string{}},
		}

		for n, tt := range addTests {
			queue := MakeTestQueue(driver, "tests/queue/next/"+strconv.Itoa(n))
			for _, msg := range tt.add {
				if err := queue.Add([]byte(msg)); err != nil {
					t.Errorf("error adding %s to %s: %s", msg, driver, err)
				}
			}

			for _, want := range tt.want {
				got, err := queue.Next()
				if err != nil {
					t.Errorf("error getting next from %s %s", driver, err)
				}
				if string(got) != want {
					t.Errorf("got %s from %s want %s", got, driver, want)
				}
			}

			if err := queue.Cleanup(); err != nil {
				t.Errorf("Error cleaning up: %s", err)
			}
		}
	}
}
