// Package mcp is dg's Model Context Protocol server (spec FR-16): the tools check, vocabulary,
// op and export over one project.
package mcp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/export"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// Server serves the tools for the project in Dir.
type Server struct {
	Dir string
}

// New returns the MCP server of the project in dir.
func New(dir string) *sdk.Server {
	s := &Server{Dir: dir}
	srv := sdk.NewServer(&sdk.Implementation{Name: "dg", Version: "0"}, &sdk.ServerOptions{
		Instructions: "dg is diagram-as-code: an HCL model (nodes, edges), views (views.hcl) and layout files. " +
			"Call vocabulary for the kinds and edge kinds, op to change the files, check to validate, and export to see a view. " +
			"Positions in op are relative to the node's frame; read drawn positions with export format json.",
	})
	for _, t := range s.Tools() {
		srv.AddTool(t.Tool, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return t.Call(ctx, req.Params.Arguments), nil
		})
	}
	return srv
}

// Tool is a tool with the function that answers a call with its JSON arguments.
type Tool struct {
	Tool *sdk.Tool
	Call func(ctx context.Context, args json.RawMessage) *sdk.CallToolResult
}

// Tools returns the server's tools.
func (s *Server) Tools() []Tool {
	var opHelp strings.Builder
	opHelp.WriteString("Apply one operation to the project's files (the same as dg op). Refused operations write nothing and return the validation errors they would add. Operations and their fields (? optional):\n")
	for _, d := range ops.Defs {
		fmt.Fprintf(&opHelp, "- %s {%s}: %s\n", d.Name, d.Signature(), d.Summary)
	}
	return []Tool{
		{&sdk.Tool{
			Name:        "check",
			Description: "Validate the project (the same as dg check --json): a JSON array of diagnostics with severity, summary, detail, file, line and column.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}, s.check},
		{&sdk.Tool{
			Name:        "vocabulary",
			Description: "The kinds (with their ports, properties, container and rows flags), edge kinds, tags and views the packs, model and views define, as JSON (the same as dg vocabulary).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}, s.vocabulary},
		{&sdk.Tool{
			Name:        "op",
			Description: opHelp.String(),
			InputSchema: ops.Schema(),
		}, s.op},
		{&sdk.Tool{
			Name:        "export",
			Description: "Draw a view (the same as dg export): png (the default) and gif return the picture as an image, svg and json as text; json holds every node's box, port positions and edge routes in canvas coordinates, and every node's frame and rel, its position relative to the frame's content origin, which op move takes.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"view":{"type":"string","description":"view ID (default main)"},"format":{"type":"string","enum":["png","gif","svg","json"],"description":"default png"}},"additionalProperties":false}`),
		}, s.export},
	}
}

func text(s string, isErr bool) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: s}}, IsError: isErr}
}

func jsonResult(v any, isErr bool) *sdk.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return text(err.Error(), true)
	}
	return text(string(data)+"\n", isErr)
}

// decode decodes a tool's arguments, which may be absent, rejecting unknown fields.
func decode(args json.RawMessage, v any) error {
	if len(bytes.TrimSpace(args)) == 0 || string(bytes.TrimSpace(args)) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) check(_ context.Context, args json.RawMessage) *sdk.CallToolResult {
	if err := decode(args, &struct{}{}); err != nil {
		return text(err.Error(), true)
	}
	_, diags, err := project.Load(s.Dir)
	if err != nil {
		return text(err.Error(), true)
	}
	if diags == nil {
		diags = diag.Diagnostics{}
	}
	return jsonResult(diags, false)
}

func (s *Server) op(_ context.Context, args json.RawMessage) *sdk.CallToolResult {
	op, err := ops.DecodeOp(args)
	var c *ops.Change
	if err == nil {
		c, err = ops.Apply(s.Dir, op, ops.NewSession())
	}
	return jsonResult(ops.NewOutcome(c, err), err != nil)
}

func (s *Server) export(ctx context.Context, args json.RawMessage) *sdk.CallToolResult {
	var a struct {
		View   string `json:"view"`
		Format string `json:"format"`
	}
	if err := decode(args, &a); err != nil {
		return text(err.Error(), true)
	}
	a.View = cmp.Or(a.View, view.DefaultView)
	a.Format = cmp.Or(a.Format, "png")
	if !slices.Contains([]string{"svg", "png", "gif", "json"}, a.Format) {
		return text(fmt.Sprintf("unknown format %q (want png, gif, svg or json)", a.Format), true)
	}
	p, diags, err := project.Load(s.Dir)
	if err != nil {
		return text(err.Error(), true)
	}
	if p.Views[a.View] == nil {
		return text(fmt.Sprintf("view %q is not defined (views: %s)", a.View, strings.Join(slices.Sorted(maps.Keys(p.Views)), ", ")), true)
	}
	v, rdiags := p.Resolve(a.View)
	diags = append(diags, rdiags...)
	if diags.HasErrors() {
		return jsonResult(diags, true)
	}
	res, err := export.Render(ctx, v, a.Format)
	if err != nil {
		return text(err.Error(), true)
	}
	out := &sdk.CallToolResult{}
	switch a.Format {
	case "png":
		out.Content = []sdk.Content{&sdk.ImageContent{Data: res.PNG, MIMEType: "image/png"}}
	case "gif":
		out.Content = []sdk.Content{&sdk.ImageContent{Data: res.GIF, MIMEType: "image/gif"}}
	case "svg":
		out.Content = []sdk.Content{&sdk.TextContent{Text: string(res.SVG)}}
	case "json":
		out.Content = []sdk.Content{&sdk.TextContent{Text: string(res.JSON)}}
	}
	var warnings []string
	for _, d := range diags {
		if d.Severity == diag.Warning {
			warnings = append(warnings, d.String())
		}
	}
	for _, id := range res.TooSmall {
		warnings = append(warnings, p.TooSmall(a.View, id).String())
	}
	if len(warnings) > 0 {
		out.Content = append(out.Content, &sdk.TextContent{Text: strings.Join(warnings, "\n")})
	}
	return out
}

func (s *Server) vocabulary(_ context.Context, args json.RawMessage) *sdk.CallToolResult {
	if err := decode(args, &struct{}{}); err != nil {
		return text(err.Error(), true)
	}
	p, _, err := project.Load(s.Dir)
	if err != nil {
		return text(err.Error(), true)
	}
	return jsonResult(p.Vocabulary(), false)
}
