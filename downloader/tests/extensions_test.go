package downloader

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"onethinglab.com/imagedown/downloader"
)

func TestIsDataURL(t *testing.T) {
	type testCase struct {
		dataURL string
		valid   bool
	}

	var testCases = []testCase{
		{"data:,A%20brief%20note", true},
		{"data:image/gif;base64,R0l", true},
		{"data,A%20brief%20note", false},
		{"data :,A%20brief%20note", false},
		{"", false},
		{" data:,", false},
	}

	for _, test := range testCases {
		if downloader.IsDataURL(test.dataURL) != test.valid {
			t.Errorf("IsDataURL('%s') != %t", test.dataURL, test.valid)
		}
	}
}

func TestParseDataURL(t *testing.T) {
	type testCase struct {
		dataURL    string
		parsedData downloader.DataURI
	}

	var testCases = []testCase{
		{"data:image/gif;base64,XXX",
		downloader.DataURI{Type: "image", Subtype: "gif", IsBase64: true,
			Data: "XXX",
				Params: make(map[string]string)}},
		{`data:,A%20brief%20note`,
		downloader.DataURI{Type: "text", Subtype: "plain", Data: `A%20brief%20note`,
				Params: map[string]string{"charset": "US-ASCII"}}},
		{`data:text/plain;charset=iso-8859-7,%be%fg%be`,
		downloader.DataURI{Type: "text", Subtype: "plain", Data: "%be%fg%be",
				Params: map[string]string{"charset": "iso-8859-7"}}},
		{`data:application/vnd-xxx-query,select_vcount,fcol_from_fieldtable/local`,
		downloader.DataURI{Type: "application", Subtype: "vnd-xxx-query",
				Data:   "select_vcount,fcol_from_fieldtable/local",
				Params: make(map[string]string)}},
		{`data:,`, downloader.DataURI{Type: "text", Subtype: "plain", Data: ``,
					Params: map[string]string{"charset": "US-ASCII"}}},
		{`data:boo/foo;,`, downloader.DataURI{Type: "boo", Subtype: "foo", Data: "",
						Params: make(map[string]string)}},
	}

	for _, test := range testCases {
		t.Run(test.dataURL, func(t *testing.T) {
			result, _ := downloader.ParseDataURL(test.dataURL)
			if !cmp.Equal(result, test.parsedData) {
				t.Errorf("ParseDataURL return different result for input: %+v", result)
			}
		})
	}
}
