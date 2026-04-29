package tools

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type RepairArgs struct {
	Text string `json:"text" jsonschema:"description=Text that contains encoding issues or garbled characters"`
}

type RepairResult struct {
	RepairedText string `json:"repaired_text"`
}

var candidateEncodings = []encoding.Encoding{
	japanese.ShiftJIS,
	japanese.EUCJP,
	japanese.ISO2022JP,
	simplifiedchinese.GBK,
	simplifiedchinese.GB18030,
	traditionalchinese.Big5,
	korean.EUCKR,
	charmap.Windows1252,
	charmap.ISO8859_1,
}

func repairEncoding(text string) string {
	if utf8.ValidString(text) && !looksLikeMojibake(text) {
		return text
	}

	raw := []byte(text)
	for _, enc := range candidateEncodings {
		decoded, _, err := transform.Bytes(enc.NewDecoder(), raw)
		if err != nil {
			continue
		}
		if utf8.Valid(decoded) && !looksLikeMojibake(string(decoded)) {
			return string(decoded)
		}
	}

	// Strip invalid UTF-8 bytes as last resort
	var buf bytes.Buffer
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		if r != utf8.RuneError {
			buf.WriteRune(r)
		}
		i += size
	}
	return buf.String()
}

// looksLikeMojibake returns true if the string has an unusually high ratio of
// replacement characters, suggesting garbled encoding.
func looksLikeMojibake(s string) bool {
	if len(s) == 0 {
		return false
	}
	replacements := strings.Count(s, "�")
	return float64(replacements)/float64(utf8.RuneCountInString(s)) > 0.1
}

func NewRepairTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "repair_encoding",
		Description: "Fixes character encoding issues and garbled text (mojibake). Detects Shift-JIS, EUC-JP, GBK, Big5, EUC-KR, Windows-1252 and other common encodings.",
	}, func(ctx tool.Context, args RepairArgs) (RepairResult, error) {
		return RepairResult{RepairedText: repairEncoding(args.Text)}, nil
	})
}
