package plist

import (
	"encoding/hex"
	"io"
	"strconv"
	"strings"
	"time"
)

type textPlistGenerator struct {
	writer io.Writer
	format int

	quotableTable *characterSet

	indent string
	depth  int

	dictKvDelimiter, dictEntryDelimiter, arrayDelimiter []byte
}

var (
	textPlistTimeLayout = "2006-01-02 15:04:05 -0700"
	padding             = "0000"
)

func (p *textPlistGenerator) generateDocument(pval cfValue) error {
	return p.writePlistValue(pval)
}

func (p *textPlistGenerator) plistQuotedString(str string) string {
	if str == "" {
		return `""`
	}
	var sb strings.Builder
	sb.Grow(len(str) + 2) // 预分配空间
	quot := false
	for _, r := range str {
		if r > 0xFF {
			quot = true
			sb.WriteString(`\U`)
			us := strconv.FormatInt(int64(r), 16)
			sb.WriteString(padding[len(us):])
			sb.WriteString(us)
		} else if r > 0x7F {
			quot = true
			sb.WriteByte('\\')
			us := strconv.FormatInt(int64(r), 8)
			sb.WriteString(padding[1+len(us):])
			sb.WriteString(us)
		} else {
			c := uint8(r)
			if p.quotableTable.ContainsByte(c) {
				quot = true
			}

			switch c {
			case '\a':
				sb.WriteString(`\a`)
			case '\b':
				sb.WriteString(`\b`)
			case '\v':
				sb.WriteString(`\v`)
			case '\f':
				sb.WriteString(`\f`)
			case '\\':
				sb.WriteString(`\\`)
			case '"':
				sb.WriteString(`\"`)
			case '\t', '\r', '\n':
				fallthrough
			default:
				sb.WriteByte(c)
			}
		}
	}
	if quot {
		return `"` + sb.String() + `"`
	}
	return sb.String()
}

func (p *textPlistGenerator) deltaIndent(depthDelta int) {
	if depthDelta < 0 {
		p.depth--
	} else if depthDelta > 0 {
		p.depth++
	}
}

func (p *textPlistGenerator) writeIndent() error {
	if len(p.indent) == 0 {
		return nil
	}
	if _, err := p.writer.Write([]byte("\n")); err != nil {
		return err
	}
	for i := 0; i < p.depth; i++ {
		if _, err := io.WriteString(p.writer, p.indent); err != nil {
			return err
		}
	}
	return nil
}

func (p *textPlistGenerator) writePlistValue(pval cfValue) error {
	if pval == nil {
		return nil
	}

	switch pval := pval.(type) {
	case *cfDictionary:
		pval.sort()
		if _, err := p.writer.Write([]byte(`{`)); err != nil {
			return err
		}
		p.deltaIndent(1)
		for i, k := range pval.keys {
			if err := p.writeIndent(); err != nil {
				return err
			}
			if _, err := io.WriteString(p.writer, p.plistQuotedString(k)); err != nil {
				return err
			}
			if _, err := p.writer.Write(p.dictKvDelimiter); err != nil {
				return err
			}
			if err := p.writePlistValue(pval.values[i]); err != nil {
				return err
			}
			if _, err := p.writer.Write(p.dictEntryDelimiter); err != nil {
				return err
			}
		}
		p.deltaIndent(-1)
		if err := p.writeIndent(); err != nil {
			return err
		}
		if _, err := p.writer.Write([]byte(`}`)); err != nil {
			return err
		}
	case *cfArray:
		if _, err := p.writer.Write([]byte(`(`)); err != nil {
			return err
		}
		p.deltaIndent(1)
		for _, v := range pval.values {
			if err := p.writeIndent(); err != nil {
				return err
			}
			if err := p.writePlistValue(v); err != nil {
				return err
			}
			if _, err := p.writer.Write(p.arrayDelimiter); err != nil {
				return err
			}
		}
		p.deltaIndent(-1)
		if err := p.writeIndent(); err != nil {
			return err
		}
		if _, err := p.writer.Write([]byte(`)`)); err != nil {
			return err
		}
	case cfString:
		if _, err := io.WriteString(p.writer, p.plistQuotedString(string(pval))); err != nil {
			return err
		}
	case *cfNumber:
		if p.format == GNUStepFormat {
			if _, err := p.writer.Write([]byte(`<*I`)); err != nil {
				return err
			}
		}
		if pval.signed {
			if _, err := io.WriteString(p.writer, strconv.FormatInt(int64(pval.value), 10)); err != nil {
				return err
			}
		} else {
			if _, err := io.WriteString(p.writer, strconv.FormatUint(pval.value, 10)); err != nil {
				return err
			}
		}
		if p.format == GNUStepFormat {
			if _, err := p.writer.Write([]byte(`>`)); err != nil {
				return err
			}
		}
	case *cfReal:
		if p.format == GNUStepFormat {
			if _, err := p.writer.Write([]byte(`<*R`)); err != nil {
				return err
			}
		}
		// GNUstep does not differentiate between 32/64-bit floats.
		if _, err := io.WriteString(p.writer, strconv.FormatFloat(pval.value, 'g', -1, 64)); err != nil {
			return err
		}
		if p.format == GNUStepFormat {
			if _, err := p.writer.Write([]byte(`>`)); err != nil {
				return err
			}
		}
	case cfBoolean:
		if p.format == GNUStepFormat {
			if pval {
				if _, err := p.writer.Write([]byte(`<*BY>`)); err != nil {
					return err
				}
			} else {
				if _, err := p.writer.Write([]byte(`<*BN>`)); err != nil {
					return err
				}
			}
		} else {
			if pval {
				if _, err := p.writer.Write([]byte(`1`)); err != nil {
					return err
				}
			} else {
				if _, err := p.writer.Write([]byte(`0`)); err != nil {
					return err
				}
			}
		}
	case cfData:
		var hexencoded [9]byte
		var l int
		var asc = 9
		hexencoded[8] = ' '

		if _, err := p.writer.Write([]byte(`<`)); err != nil {
			return err
		}
		b := []byte(pval)
		for i := 0; i < len(b); i += 4 {
			l = i + 4
			if l >= len(b) {
				l = len(b)
				// We no longer need the space - or the rest of the buffer.
				// (we used >= above to get this part without another conditional :P)
				asc = (l - i) * 2
			}
			// Fill the buffer (only up to 8 characters, to preserve the space we implicitly include
			// at the end of every encode)
			hex.Encode(hexencoded[:8], b[i:l])
			if _, err := io.WriteString(p.writer, string(hexencoded[:asc])); err != nil {
				return err
			}
		}
		if _, err := p.writer.Write([]byte(`>`)); err != nil {
			return err
		}
	case cfDate:
		if p.format == GNUStepFormat {
			if _, err := p.writer.Write([]byte(`<*D`)); err != nil {
				return err
			}
			if _, err := io.WriteString(p.writer, time.Time(pval).In(time.UTC).Format(textPlistTimeLayout)); err != nil {
				return err
			}
			if _, err := p.writer.Write([]byte(`>`)); err != nil {
				return err
			}
		} else {
			if _, err := io.WriteString(p.writer, p.plistQuotedString(time.Time(pval).In(time.UTC).Format(textPlistTimeLayout))); err != nil {
				return err
			}
		}
	case cfUID:
		return p.writePlistValue(pval.toDict())
	}
	return nil
}

func (p *textPlistGenerator) Indent(i string) {
	p.indent = i
	if i == "" {
		p.dictKvDelimiter = []byte(`=`)
	} else {
		// For pretty-printing
		p.dictKvDelimiter = []byte(` = `)
	}
}

func newTextPlistGenerator(w io.Writer, format int) *textPlistGenerator {
	table := &osQuotable
	if format == GNUStepFormat {
		table = &gsQuotable
	}
	return &textPlistGenerator{
		writer:             w,
		format:             format,
		quotableTable:      table,
		dictKvDelimiter:    []byte(`=`),
		arrayDelimiter:     []byte(`,`),
		dictEntryDelimiter: []byte(`;`),
	}
}
