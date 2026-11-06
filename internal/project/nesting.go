package project

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/nvcnvn/dg/internal/spec"
)

// maxNesting bounds how far HCL may nest, and how many operators, indexes and dots one statement
// may chain. Parsing and evaluating recurse that deep, and in WebAssembly a few hundred levels
// overflow the browser's stack, which kills Go with nothing to recover in.
const maxNesting = 100

// TooDeep returns an error when src goes past maxNesting.
func TooDeep(src []byte, name string) hcl.Diagnostics {
	tokens, _ := hclsyntax.LexConfig(src, name, hcl.InitialPos)
	var depth, groups, unary, chain int
	for _, tok := range tokens {
		switch tok.Type {
		case hclsyntax.TokenOParen, hclsyntax.TokenOBrack, hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			depth++
			groups++
			if tok.Type == hclsyntax.TokenOBrack {
				chain++
			}
		case hclsyntax.TokenCParen, hclsyntax.TokenCBrack, hclsyntax.TokenTemplateSeqEnd:
			depth--
			groups--
		case hclsyntax.TokenOBrace:
			depth++
		case hclsyntax.TokenCBrace:
			depth--
		case hclsyntax.TokenComma:
			chain = 0
		case hclsyntax.TokenNewline:
			// HCL ignores newlines within brackets, so a statement goes on there.
			if groups <= 0 {
				chain = 0
			}
			continue
		case hclsyntax.TokenBang, hclsyntax.TokenMinus:
			unary++
			chain++
		case hclsyntax.TokenPlus, hclsyntax.TokenStar, hclsyntax.TokenSlash, hclsyntax.TokenPercent,
			hclsyntax.TokenEqualOp, hclsyntax.TokenNotEqual, hclsyntax.TokenLessThan, hclsyntax.TokenLessThanEq,
			hclsyntax.TokenGreaterThan, hclsyntax.TokenGreaterThanEq, hclsyntax.TokenAnd, hclsyntax.TokenOr,
			hclsyntax.TokenQuestion, hclsyntax.TokenDot:
			chain++
		}
		if tok.Type != hclsyntax.TokenBang && tok.Type != hclsyntax.TokenMinus {
			unary = 0
		}
		if depth > maxNesting || unary > maxNesting || chain > maxNesting {
			return hcl.Diagnostics{spec.Errorf(tok.Range, "Nested too deeply", "This goes past %d levels, or %d operators in one statement.", maxNesting, maxNesting)}
		}
	}
	return nil
}
