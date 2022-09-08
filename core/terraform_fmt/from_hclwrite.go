package terraform_fmt

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"unicode"
	"unicode/utf8"
)

//this is originally a private function from hclwrite (hclwrite.escapeQuotedStringLit)
func escapeQuotedStringLit(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	buf := make([]byte, 0, len(s))
	for i, r := range s {
		switch r {
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		case '"':
			buf = append(buf, '\\', '"')
		case '\\':
			buf = append(buf, '\\', '\\')
		case '$', '%':
			buf = appendRune(buf, r)
			remain := s[i+1:]
			if len(remain) > 0 && remain[0] == '{' {
				// Double up our template introducer symbol to escape it.
				buf = appendRune(buf, r)
			}
		default:
			if !unicode.IsPrint(r) {
				var fmted string
				if r < 65536 {
					fmted = fmt.Sprintf("\\u%04x", r)
				} else {
					fmted = fmt.Sprintf("\\U%08x", r)
				}
				buf = append(buf, fmted...)
			} else {
				buf = appendRune(buf, r)
			}
		}
	}
	return buf
}
func appendRune(b []byte, r rune) []byte {
	l := utf8.RuneLen(r)
	for i := 0; i < l; i++ {
		b = append(b, 0) // make room at the end of our buffer
	}
	ch := b[len(b)-l:]
	utf8.EncodeRune(ch, r)
	return b
}

//this is originally a private function from hclwrite (hclwrite.appendTokensForTraversal)
func appendTokensForTraversal(traversal hcl.Traversal, toks hclwrite.Tokens) hclwrite.Tokens {
	for _, step := range traversal {
		toks = appendTokensForTraversalStep(step, toks)
	}
	return toks
}

func appendTokensForTraversalStep(step hcl.Traverser, toks hclwrite.Tokens) hclwrite.Tokens {
	switch ts := step.(type) {
	case hcl.TraverseRoot:
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenIdent,
			Bytes: []byte(ts.Name),
		})
	case hcl.TraverseAttr:
		toks = append(
			toks,
			&hclwrite.Token{
				Type:  hclsyntax.TokenDot,
				Bytes: []byte{'.'},
			},
			&hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(ts.Name),
			},
		)
	case hcl.TraverseIndex:
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrack,
			Bytes: []byte{'['},
		})
		toks = appendTokensForValue(ts.Key, toks)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrack,
			Bytes: []byte{']'},
		})
	default:
		panic(fmt.Sprintf("unsupported traversal step type %T", step))
	}

	return toks
}

func appendTokensForValue(val cty.Value, toks hclwrite.Tokens) hclwrite.Tokens {
	switch {

	case !val.IsKnown():
		panic("cannot produce tokens for unknown value")

	case val.IsNull():
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenIdent,
			Bytes: []byte(`null`),
		})

	case val.Type() == cty.Bool:
		var src []byte
		if val.True() {
			src = []byte(`true`)
		} else {
			src = []byte(`false`)
		}
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenIdent,
			Bytes: src,
		})

	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		srcStr := bf.Text('f', -1)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenNumberLit,
			Bytes: []byte(srcStr),
		})

	case val.Type() == cty.String:
		// TODO: If it's a multi-line string ending in a newline, format
		// it as a HEREDOC instead.
		src := escapeQuotedStringLit(val.AsString())
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOQuote,
			Bytes: []byte{'"'},
		})
		if len(src) > 0 {
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenQuotedLit,
				Bytes: src,
			})
		}
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCQuote,
			Bytes: []byte{'"'},
		})

	case val.Type().IsListType() || val.Type().IsSetType() || val.Type().IsTupleType():
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrack,
			Bytes: []byte{'['},
		})

		i := 0
		for it := val.ElementIterator(); it.Next(); {
			if i > 0 {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenComma,
					Bytes: []byte{','},
				})
			}
			_, eVal := it.Element()
			toks = appendTokensForValue(eVal, toks)
			i++
		}

		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrack,
			Bytes: []byte{']'},
		})

	case val.Type().IsMapType() || val.Type().IsObjectType():
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrace,
			Bytes: []byte{'{'},
		})
		if val.LengthInt() > 0 {
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenNewline,
				Bytes: []byte{'\n'},
			})
		}

		i := 0
		for it := val.ElementIterator(); it.Next(); {
			eKey, eVal := it.Element()
			if hclsyntax.ValidIdentifier(eKey.AsString()) {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenIdent,
					Bytes: []byte(eKey.AsString()),
				})
			} else {
				toks = appendTokensForValue(eKey, toks)
			}
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenEqual,
				Bytes: []byte{'='},
			})
			toks = appendTokensForValue(eVal, toks)
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenNewline,
				Bytes: []byte{'\n'},
			})
			i++
		}

		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrace,
			Bytes: []byte{'}'},
		})

	default:
		panic(fmt.Sprintf("cannot produce tokens for %#v", val))
	}

	return toks
}
