package daytona

import "testing"

func TestParseOutputFileLine(t *testing.T) {
	cases := []struct {
		line string
		want WorkspaceOutputFile
		ok   bool
	}{
		{
			line: "1234 d41d8cd98f00b204e9800998ecf8427e  out/result.png",
			want: WorkspaceOutputFile{Rel: "out/result.png", Size: 1234, MD5: "d41d8cd98f00b204e9800998ecf8427e"},
			ok:   true,
		},
		{
			// md5sum binary-mode separator and a path with spaces
			line: "7 d41d8cd98f00b204e9800998ecf8427e *out/my report.csv",
			want: WorkspaceOutputFile{Rel: "out/my report.csv", Size: 7, MD5: "d41d8cd98f00b204e9800998ecf8427e"},
			ok:   true,
		},
		{line: "", ok: false},
		{line: "notanumber d41d8cd98f00b204e9800998ecf8427e  out/x", ok: false},
		{line: "12 zzzz8cd98f00b204e9800998ecf8427e  out/x", ok: false},
		{line: "12 d41d8cd98f00b204e9800998ecf8427e  work/x", ok: false}, // outside out/
		{line: "12 d41d8cd98f00b204e9800998ecf8427e  ../etc/passwd", ok: false},
	}
	for _, c := range cases {
		got, ok := parseOutputFileLine(c.line)
		if ok != c.ok || got != c.want {
			t.Errorf("parseOutputFileLine(%q) = (%+v, %v), want (%+v, %v)",
				c.line, got, ok, c.want, c.ok)
		}
	}
}
