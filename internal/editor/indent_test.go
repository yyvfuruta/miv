package editor

import "testing"

func TestYAMLNewLineIndent(t *testing.T) {
	for _, tt := range []struct{ name, text, indent string }{
		{"mapping", "services:", "  "},
		{"nested mapping", "services:\n  api:", "    "},
		{"mapping comment", "services: # applications", "  "},
		{"unicode key", "café:", "  "},
		{"scalar value", "  port: 8080", "  "},
		{"quoted colon", "  text: \"name:\"", "  "},
		{"quoted hash", "  \"key # label\": # comment", "    "},
		{"escaped quote", `  value: "say \"hello:\""`, "  "},
		{"single quote", "  value: 'it''s a key:'", "  "},
		{"plain apostrophe", "  - name: don't stop", "    "},
		{"unfinished quote", "  value: \"unfinished:", "  "},
		{"url", "  url: https://example.com/path#anchor", "  "},
		{"comment", "  # key:", "  "},
		{"blank", "    ", "    "},
		{"sequence mapping", "  - name: api", "    "},
		{"sequence nested mapping", "  - config:", "      "},
		{"sequence scalar", "  - item", "  "},
		{"empty sequence item", "  -", "    "},
		{"flow collection", "  values: {key: value}", "  "},
		{"unfinished flow", "  values: [first,", "  "},
		{"literal scalar header", "  script: |", "    "},
		{"folded scalar header", "  description: >- # folded", "    "},
		{"explicit scalar indent", "  script: |4-", "      "},
		{"sequence scalar header", "  - |", "    "},
		{"scalar content", "script: |\n  this ends with a colon:", "  "},
		{"scalar comment content", "script: |\n  # literal text\n  key:", "  "},
		{"list scalar content", "- script: |\n    literal:", "    "},
		{"scalar indented content", "script: |\n  literal:\n    more:", "    "},
		{"field after list scalar", "- script: |\n    literal\n  config:", "    "},
		{"nested field after scalar", "root:\n  script: |\n    literal\n  config:\n    child:", "      "},
		{"after scalar", "script: |\n  literal:\nnext:", "  "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"<Enter>", "o"} {
				e := fixture(tt.text)
				e.Buffer().Path = "config.yaml"
				if key == "o" {
					keys(e, "Go")
				} else {
					keys(e, "GA")
					e.Handle(key)
				}
				assertText(t, e, tt.text+"\n"+tt.indent)
				if e.Mode != Insert || e.Buffer().Cursor != len(e.Buffer().Text) {
					t.Fatal("cursor not ready for input")
				}
				e.Handle("<Esc>")
				keys(e, "u")
				assertText(t, e, tt.text)
			}
		})
	}
}

func TestIndentFileTypesAndLineSplits(t *testing.T) {
	for _, tt := range []struct{ path, indent string }{{"config.yml", "    "}, {"config.YAML", "    "}, {"config.json", "  "}, {"main.go", "  "}, {"notes.txt", "  "}} {
		e := fixture("  key:")
		e.Buffer().Path = tt.path
		keys(e, "A")
		e.Handle("<Enter>")
		assertText(t, e, "  key:\n"+tt.indent)
	}
	e := fixture("  key: value")
	e.Buffer().Path = "config.yaml"
	keys(e, "02li")
	e.Handle("<Enter>")
	assertText(t, e, "  \n  key: value")
	e = fixture("  key:")
	e.Buffer().Path = "config.yaml"
	keys(e, "O")
	assertText(t, e, "  \n  key:")
	e = fixture("    key:")
	e.Buffer().Path = "config.yaml"
	keys(e, "li")
	e.Handle("<Enter>")
	assertText(t, e, " \n    key:")
}

func TestInsertDedent(t *testing.T) {
	for _, key := range []string{"<C-d>", "<S-Tab>"} {
		e := fixture("root:")
		e.Buffer().Path = "config.yaml"
		keys(e, "A")
		e.Handle("<Enter>")
		keys(e, "child:")
		e.Handle("<Enter>")
		assertText(t, e, "root:\n  child:\n    ")
		e.Handle(key)
		assertText(t, e, "root:\n  child:\n  ")
		e.Handle(key)
		e.Handle(key)
		assertText(t, e, "root:\n  child:\n")
		keys(e, "next: value")
		e.Handle("<Esc>")
		keys(e, "u")
		assertText(t, e, "root:")
		e = fixture("    text")
		e.Buffer().Path = "main.go"
		keys(e, "A")
		e.Handle(key)
		assertText(t, e, "text")
		if e.Buffer().Cursor != 4 {
			t.Fatal("dedent moved cursor away from text")
		}
		e = fixture("\ttext")
		keys(e, "i")
		e.Handle(key)
		assertText(t, e, "text")
		if e.Buffer().Cursor != 0 {
			t.Fatal("dedent produced invalid cursor")
		}
	}
}

func TestYAMLIndentRepeatAndPaste(t *testing.T) {
	e := fixture("root:")
	e.Buffer().Path = "config.yml"
	keys(e, "ochild:")
	e.Handle("<Esc>")
	keys(e, ".")
	assertText(t, e, "root:\n  child:\n    child:")
	e = fixture("")
	e.Buffer().Path = "config.yaml"
	keys(e, "i")
	e.InsertPaste("root:\nchild: value\n- item")
	assertText(t, e, "root:\nchild: value\n- item")
}
