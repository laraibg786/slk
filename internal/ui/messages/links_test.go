package messages

import (
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []Link
	}{
		{
			name: "no links",
			text: "just some text with <@U123> and <#C123|general>",
			want: nil,
		},
		{
			name: "bare link",
			text: "see <https://example.com/page>",
			want: []Link{{URL: "https://example.com/page"}},
		},
		{
			name: "labeled link",
			text: "see <https://example.com/page|the docs>",
			want: []Link{{URL: "https://example.com/page", Label: "the docs"}},
		},
		{
			name: "mixed, order of appearance",
			text: "<https://a.example/1|first> then <https://b.example/2>",
			want: []Link{
				{URL: "https://a.example/1", Label: "first"},
				{URL: "https://b.example/2"},
			},
		},
		{
			name: "duplicate URLs deduped",
			text: "<https://a.example/1> and again <https://a.example/1|same>",
			want: []Link{{URL: "https://a.example/1"}},
		},
		{
			name: "mailto",
			text: "<mailto:foo@example.com|foo@example.com>",
			want: []Link{{URL: "mailto:foo@example.com", Label: "foo@example.com"}},
		},
		{name: "http", text: "<http://example.com/a>", want: []Link{{URL: "http://example.com/a"}}},
		{name: "ftp labeled", text: "<ftp://example.com/f|files>", want: []Link{{URL: "ftp://example.com/f", Label: "files"}}},
		{name: "file", text: "<file:///tmp/notes.txt>", want: []Link{{URL: "file:///tmp/notes.txt"}}},
		{name: "tel", text: "<tel:+15550100|call>", want: []Link{{URL: "tel:+15550100", Label: "call"}}},
		{name: "ssh", text: "<ssh://host.example>", want: []Link{{URL: "ssh://host.example"}}},
		{name: "javascript refused", text: "<javascript:alert(1)|x> <javascript:alert(1)>", want: nil},
		{name: "data refused", text: "<data:text/html;base64,AAAA>", want: nil},
		{name: "vbscript refused", text: "<vbscript:msgbox(1)|x>", want: nil},
		{name: "channel mention", text: "<#CFAKE0001|general>", want: nil},
		{name: "user mention", text: "<@UFAKE0001>", want: nil},
		{name: "broadcast", text: "<!here> <!subteam^SFAKE0001|@team>", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractLinks(tc.text)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExtractLinks(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
		})
	}
}
