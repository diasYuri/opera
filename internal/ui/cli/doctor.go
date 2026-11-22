package cli

import (
	"fmt"
	"strings"

	"github.com/diasYuri/opera/internal/core"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/diagnostics"
)

func (a App) doctor(cfg config.Resolved, jsonl bool) int {
	report := struct {
		diagnostics.Report
		Composition []core.CompositionSummary `json:"composition"`
	}{Report: diagnostics.Collect(cfg, a.LookPath)}
	for _, profile := range []core.ProfileID{core.ProfileDefault, core.ProfileHeadless, core.ProfileInspection} {
		graph, err := core.DescribeComposition(cfg, core.Options{Profile: profile, ProviderCatalog: a.ProviderCatalog, Thinking: cfg.String("thinking")})
		if err != nil {
			report.Checks = append(report.Checks, diagnostics.Check{
				Name: "composition." + string(profile), Status: "error",
				Detail:   "The selected graph or capability configuration is invalid.",
				Fallback: "Check provider and search selections in your configuration.",
			})
			continue
		}
		report.Composition = append(report.Composition, graph)
	}
	code := ExitOK
	for _, check := range report.Checks {
		if check.Status == "error" {
			code = ExitFailure
		}
	}
	if jsonl {
		if result := a.emit("doctor.report", report); result != ExitOK {
			return result
		}
		return code
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Opera foundation diagnostics\nGo: %s; Y SDK: %s; credentials: %s\n", report.Go, report.SDK, report.CredentialStore)
	for _, check := range report.Checks {
		fmt.Fprintf(&out, "[%s] %s: %s\n", check.Status, check.Name, check.Detail)
		if check.Fallback != "" {
			fmt.Fprintln(&out, "  "+check.Fallback)
		}
	}
	for _, graph := range report.Composition {
		fmt.Fprintf(&out, "Composition %s\n", graph.Profile)
		if graph.Provider != "" {
			fmt.Fprintf(&out, "  Provider %s; search %s\n", graph.Provider, graph.Search)
			fmt.Fprintf(&out, "  Providers: %s\n  Search providers: %s\n  Tool packs: %s\n", strings.Join(graph.ProviderIDs, ", "), strings.Join(graph.SearchProviders, ", "), strings.Join(graph.ToolPacks, ", "))
		}
		for _, piece := range graph.Components {
			fmt.Fprintf(&out, "  %s@%s", piece.ID, piece.Version)
			if piece.Slot != "" {
				fmt.Fprintf(&out, " [%s]", piece.Slot)
			}
			fmt.Fprintf(&out, " (%s)\n", strings.Join(piece.Capabilities, ", "))
			for _, service := range piece.Provides {
				fmt.Fprintf(&out, "    provides %s@%s\n", service.ID, service.Version)
			}
			for _, service := range piece.Requires {
				fmt.Fprintf(&out, "    requires %s@%s\n", service.ID, service.Version)
			}
			for _, binding := range piece.Bindings {
				fmt.Fprintf(&out, "    binds %s from %s\n", binding.ServiceID, binding.ProviderID)
			}
			for _, field := range piece.Config {
				fmt.Fprintf(&out, "    %s (%s)\n", field.Key, field.Source)
			}
		}
	}
	if result := a.writeText(out.String()); result != ExitOK {
		return result
	}
	return code
}
