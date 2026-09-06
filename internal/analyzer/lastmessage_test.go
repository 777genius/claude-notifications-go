package analyzer

import "testing"

func TestClassifyLastMessage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Status
	}{
		{name: "plain completion", text: "Done, all tests pass.", want: StatusTaskComplete},
		{name: "empty", text: "", want: StatusTaskComplete},
		{name: "whitespace only", text: "  \n\t ", want: StatusTaskComplete},
		{name: "question", text: "Should I also update the README?", want: StatusQuestion},
		{name: "question with trailing space", text: "Which option do you prefer? ", want: StatusQuestion},
		{name: "question mark inside", text: "Fixed the what? handling in the parser", want: StatusTaskComplete},
		{name: "multibyte", text: "Готово ✅", want: StatusTaskComplete},
		{name: "multibyte question", text: "Продолжать?", want: StatusQuestion},
	}
	for _, tc := range cases {
		if got := ClassifyLastMessage(tc.text); got != tc.want {
			t.Errorf("%s: ClassifyLastMessage(%q) = %q, want %q", tc.name, tc.text, got, tc.want)
		}
	}
}
