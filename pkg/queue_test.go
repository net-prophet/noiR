package pkg

import (
	"strconv"
	"testing"
)

func TestQueueAdd(t *testing.T) {
	addTests := []struct {
		add  []string
		want int64
	}{
		{[]string{"a"}, 1},
		{[]string{"a", "b", "c"}, 3},
		{[]string{}, 0},
	}

	for n, tt := range addTests {
		queue := MakeTestQueue("tests/queue/add/"+strconv.Itoa(n))

		for _, msg := range tt.add {
			if err := queue.Add([]byte(msg)); err != nil {
				t.Errorf("Error adding %s to queue: %s", msg, err)

			}
		}
		got, err := queue.Count()
		if err != nil {
			t.Errorf("error getting count %s", err)
		}
		if got != tt.want {
			t.Errorf("got %d want %d", got, tt.want)
		}

		if err := queue.Cleanup(); err != nil {
			t.Errorf("Error cleaning up: %s", err)
		}

	}
}

func TestQueueNext(t *testing.T) {

	addTests := []struct {
		add  []string
		want []string
	}{
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{}, []string{}},
	}

	for n, tt := range addTests {
		queue := MakeTestQueue("tests/queue/next/"+strconv.Itoa(n))
		for _, msg := range tt.add {
			if err := queue.Add([]byte(msg)); err != nil {
				t.Errorf("Error adding %s to queue: %s", msg, err)
			}
		}

		for _, want := range tt.want {
			got, err := queue.Next()
			if err != nil {
				t.Errorf("Error getting next %s", err)
			}
			if string(got) != want {
				t.Errorf("got %s want %s", got, want)
			}
		}

		if err := queue.Cleanup(); err != nil {
			t.Errorf("Error cleaning up: %s", err)
		}

	}
}
