package reggen

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

type testCase struct {
	regex string
}

var cases = []testCase{
	{`123[0-2]+.*\w{3}`},
	{`^\d{1,2}[/](1[0-2]|[1-9])[/]((19|20)\d{2})$`},
	{`^((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])$`},
	{`^\d+$`},
	{`\D{3}`},
	{`((123)?){3}`},
	{`(ab|bc)def`},
	{`[^abcdef]{5}`},
	{`[^1]{3,5}`},
	{`[[:upper:]]{5}`},
	{`[^0-5a-z\s]{5}`},
	{`Z{2,5}`},
	{`[a-zA-Z]{100}`},
	{`^[a-z]{5,10}@[a-z]{5,10}\.(com|net|org)$`},
	{"^(([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])(%[\\p{N}\\p{L}]+)?$"},
	{`\p{N}\p{L}\p{Han}`},
}

func TestGenerate(t *testing.T) {
	for _, test := range cases {
		for i := 0; i < 10; i++ {
			r, err := NewGenerator(test.regex)
			if err != nil {
				t.Fatal("Error creating generator: ", err)
			}
			r.debug = false
			res := r.Generate(50)
			// only print first result
			if i < 1 {
				fmt.Printf("Regex: %v Result: \"%s\"\n", test.regex, res)
			}
			re, err := regexp.Compile(test.regex)
			if err != nil {
				t.Fatal("Invalid test case. regex: ", test.regex, " failed to compile:", err)
			}
			if !re.MatchString(res) {
				t.Error("Generated data does not match regex. Regex: ", test.regex, " output: ", res)
			}
		}
	}
}

func TestSeed(t *testing.T) {
	g1, err := NewGenerator(cases[0].regex)
	if err != nil {
		t.Fatal("Error creating generator: ", err)
	}
	g2, err := NewGenerator(cases[0].regex)
	if err != nil {
		t.Fatal("Error creating generator: ", err)
	}
	currentTime := time.Now().UnixNano()
	g1.SetSeed(currentTime)
	g2.SetSeed(currentTime)
	for i := 0; i < 10; i++ {
		if g1.Generate(100) != g2.Generate(100) {
			t.Error("Results are not reproducible")
		}
	}

	g1.SetSeed(123)
	g2.SetSeed(456)
	for i := 0; i < 10; i++ {
		if g1.Generate(100) == g2.Generate(100) {
			t.Error("Results should not match")
		}
	}

}

func BenchmarkGenerate(b *testing.B) {
	r, err := NewGenerator(`^[a-z]{5,10}@[a-z]+\.(com|net|org)$`)
	if err != nil {
		b.Fatal("Error creating generator: ", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Generate(10)
	}
}
