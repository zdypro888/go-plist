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

func (p *xmlPlistGenerator) generateDocument(root cfValue) {
	p.WriteString(xmlHEADER)
	p.WriteString(xmlDOCTYPE)

	p.WriteString(fmt.Sprintf("<%s version=\"1.0\">\n", xmlPlistTag))
	p.writePlistValue(root)
	p.WriteString(fmt.Sprintf("</%s>", xmlPlistTag))
	p.Flush()
}

func (p *xmlPlistGenerator) element(key string, value string) {
	p.writeIndent()
	if len(value) == 0 {
		p.WriteString(fmt.Sprintf("<%s/>\n", key))
	} else {
		p.WriteString(fmt.Sprintf("<%s>", key))
		err := xml.EscapeText(p.Writer, []byte(value))
		if err != nil {
			panic(err)
		}
		p.WriteString(fmt.Sprintf("</%s>\n", key))
	}
}

func (p *xmlPlistGenerator) writeDictionary(dict *cfDictionary) {
	dict.sort()
	if len(dict.keys) == 0 {
		p.writeIndent()
		p.WriteString(fmt.Sprintf("<%s/>\n", xmlDictTag))
	} else {
		p.writeIndent()
		p.WriteString(fmt.Sprintf("<%s>\n", xmlDictTag))
		p.depth++
		for i, k := range dict.keys {
			p.writeIndent()
			p.WriteString(fmt.Sprintf("<%s>%s</%s>\n", xmlKeyTag, k, xmlKeyTag))

			p.writePlistValue(dict.values[i])
		}
		p.depth--
		p.writeIndent()
		p.WriteString(fmt.Sprintf("</%s>\n", xmlDictTag))
	}
}

func (p *xmlPlistGenerator) writeArray(a *cfArray) {
	if len(a.values) == 0 {
		p.writeIndent()
		p.WriteString(fmt.Sprintf("<%s/>\n", xmlArrayTag))
	} else {
		p.writeIndent()
		p.WriteString(fmt.Sprintf("<%s>\n", xmlArrayTag))
		p.depth++
		for _, v := range a.values {
			p.writePlistValue(v)
		}
		p.depth--
		p.writeIndent()
		p.WriteString(fmt.Sprintf("</%s>\n", xmlArrayTag))
	}
}

func (p *xmlPlistGenerator) writePlistValue(pval cfValue) {
	if pval == nil {
		return
	}
	switch pval := pval.(type) {
	case cfString:
		p.element(xmlStringTag, string(pval))
	case *cfNumber:
		if pval.signed {
			p.element(xmlIntegerTag, strconv.FormatInt(int64(pval.value), 10))
		} else {
			p.element(xmlIntegerTag, strconv.FormatUint(pval.value, 10))
		}
	case *cfReal:
		p.element(xmlRealTag, formatXMLFloat(pval.value))
	case cfBoolean:
		if bool(pval) {
			p.element(xmlTrueTag, "")
		} else {
			p.element(xmlFalseTag, "")
		}
	case cfData:
		dataBase64 := base64.StdEncoding.EncodeToString([]byte(pval))
		if len(dataBase64) > 68 {
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
			p.element(xmlDataTag, dataBase64)
		}
	case cfDate:
		p.element(xmlDateTag, time.Time(pval).In(time.UTC).Format(time.RFC3339))
	case *cfDictionary:
		p.writeDictionary(pval)
	case *cfArray:
		p.writeArray(pval)
	case cfUID:
		p.writePlistValue(pval.toDict())
	}
}

func newXMLPlistGenerator(w io.Writer) *xmlPlistGenerator {
	return &xmlPlistGenerator{Writer: bufio.NewWriter(w)}
}
