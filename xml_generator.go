package plist

import (
	"bufio"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

const (
	xmlHEADER     string = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
	xmlDOCTYPE           = `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n"
	xmlArrayTag          = "array"
	xmlDataTag           = "data"
	xmlDateTag           = "date"
	xmlDictTag           = "dict"
	xmlFalseTag          = "false"
	xmlIntegerTag        = "integer"
	xmlKeyTag            = "key"
	xmlPlistTag          = "plist"
	xmlRealTag           = "real"
	xmlStringTag         = "string"
	xmlTrueTag           = "true"
)

func formatXMLFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

type xmlPlistGenerator struct {
	*bufio.Writer

	indent     string
	depth      int
	putNewline bool
}

func (p *xmlPlistGenerator) Indent(i string) {
	p.indent = i
}

func (p *xmlPlistGenerator) writeIndent() {
	for i := 0; i < p.depth; i++ {
		p.WriteString(p.indent)
	}
}

func (p *xmlPlistGenerator) generateDocument(root cfValue) error {
	p.WriteString(xmlHEADER)
	p.WriteString(xmlDOCTYPE)

	if p.indent != "" {
		p.WriteString(fmt.Sprintf("<%s version=\"1.0\">\n", xmlPlistTag))
		p.depth = 1 // Start with depth 1 for content inside plist
	} else {
		p.WriteString(fmt.Sprintf("<%s version=\"1.0\">", xmlPlistTag))
	}
	if err := p.writePlistValue(root); err != nil {
		return err
	}
	if p.indent != "" {
		p.WriteString(fmt.Sprintf("</%s>\n", xmlPlistTag))
	} else {
		p.WriteString(fmt.Sprintf("</%s>", xmlPlistTag))
	}
	return p.Flush()
}

func (p *xmlPlistGenerator) element(key string, value string) error {
	p.writeIndent()
	if len(value) == 0 {
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("<%s/>\n", key))
		} else {
			p.WriteString(fmt.Sprintf("<%s/>", key))
		}
	} else {
		p.WriteString(fmt.Sprintf("<%s>", key))
		if err := xml.EscapeText(p.Writer, []byte(value)); err != nil {
			return err
		}
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("</%s>\n", key))
		} else {
			p.WriteString(fmt.Sprintf("</%s>", key))
		}
	}
	return nil
}

func (p *xmlPlistGenerator) writeDictionary(dict *cfDictionary) error {
	dict.sort()
	if len(dict.keys) == 0 {
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("<%s/>\n", xmlDictTag))
		} else {
			p.WriteString(fmt.Sprintf("<%s/>", xmlDictTag))
		}
	} else {
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("<%s>\n", xmlDictTag))
		} else {
			p.WriteString(fmt.Sprintf("<%s>", xmlDictTag))
		}
		p.depth++
		for i, k := range dict.keys {
			p.writeIndent()
			if k == "" {
				if p.indent != "" {
					p.WriteString(fmt.Sprintf("<%s/>\n", xmlKeyTag))
				} else {
					p.WriteString(fmt.Sprintf("<%s/>", xmlKeyTag))
				}
			} else {
				if p.indent != "" {
					p.WriteString(fmt.Sprintf("<%s>%s</%s>\n", xmlKeyTag, k, xmlKeyTag))
				} else {
					p.WriteString(fmt.Sprintf("<%s>%s</%s>", xmlKeyTag, k, xmlKeyTag))
				}
			}

			if err := p.writePlistValue(dict.values[i]); err != nil {
				return err
			}
		}
		p.depth--
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("</%s>\n", xmlDictTag))
		} else {
			p.WriteString(fmt.Sprintf("</%s>", xmlDictTag))
		}
	}
	return nil
}

func (p *xmlPlistGenerator) writeArray(a *cfArray) error {
	if len(a.values) == 0 {
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("<%s/>\n", xmlArrayTag))
		} else {
			p.WriteString(fmt.Sprintf("<%s/>", xmlArrayTag))
		}
	} else {
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("<%s>\n", xmlArrayTag))
		} else {
			p.WriteString(fmt.Sprintf("<%s>", xmlArrayTag))
		}
		p.depth++
		for _, v := range a.values {
			if err := p.writePlistValue(v); err != nil {
				return err
			}
		}
		p.depth--
		p.writeIndent()
		if p.indent != "" {
			p.WriteString(fmt.Sprintf("</%s>\n", xmlArrayTag))
		} else {
			p.WriteString(fmt.Sprintf("</%s>", xmlArrayTag))
		}
	}
	return nil
}

func (p *xmlPlistGenerator) writePlistValue(pval cfValue) error {
	if pval == nil {
		return nil
	}
	switch pval := pval.(type) {
	case cfString:
		return p.element(xmlStringTag, string(pval))
	case *cfNumber:
		if pval.signed {
			return p.element(xmlIntegerTag, strconv.FormatInt(int64(pval.value), 10))
		} else {
			return p.element(xmlIntegerTag, strconv.FormatUint(pval.value, 10))
		}
	case *cfReal:
		return p.element(xmlRealTag, formatXMLFloat(pval.value))
	case cfBoolean:
		if bool(pval) {
			return p.element(xmlTrueTag, "")
		} else {
			return p.element(xmlFalseTag, "")
		}
	case cfData:
		dataBase64 := base64.StdEncoding.EncodeToString([]byte(pval))
		if len(dataBase64) > 68 && p.indent != "" {
			p.writeIndent()
			p.WriteString(fmt.Sprintf("<%s>\n", xmlDataTag))
			for i := 0; i < len(dataBase64); i += 68 {
				p.writeIndent()
				endoff := i + 68
				if endoff > len(dataBase64) {
					endoff = len(dataBase64)
				}
				p.WriteString(dataBase64[i:endoff])
				p.WriteString("\n")
			}
			p.writeIndent()
			p.WriteString(fmt.Sprintf("</%s>\n", xmlDataTag))

		} else {
			return p.element(xmlDataTag, dataBase64)
		}
	case cfDate:
		return p.element(xmlDateTag, time.Time(pval).In(time.UTC).Format(time.RFC3339))
	case *cfDictionary:
		return p.writeDictionary(pval)
	case *cfArray:
		return p.writeArray(pval)
	case cfUID:
		return p.writePlistValue(pval.toDict())
	}
	return nil
}

func newXMLPlistGenerator(w io.Writer) *xmlPlistGenerator {
	return &xmlPlistGenerator{Writer: bufio.NewWriter(w)}
}
