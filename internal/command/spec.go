package command

import (
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/spf13/cobra"
)

// Registrar builds a resource's top-level command. Resource files register one
// via Register() (typically in init()), and root iterates Commands(); root.go
// stays constant regardless of how many resources exist.
type Registrar func() *cobra.Command

var registry []Registrar

// Register adds a resource command builder to the registry.
func Register(r Registrar) { registry = append(registry, r) }

// Commands materialises all registered resource commands.
func Commands() []*cobra.Command {
	out := make([]*cobra.Command, 0, len(registry))
	for _, r := range registry {
		out = append(out, r())
	}
	return out
}

// Spec declares a standard resource. Only List and Get are generic verb-builders
// today (they cover the bulk of the read-surface); search/create/update/delete
// and all non-standard ops (revisions, transitions, restrictions, ...) are added
// as hand-written subcommands via Extra, still using Build()+RenderError so they
// share the same deps/error/render flow. F3 may promote a verb to a builder if
// the pattern repeats often enough to be worth it.
type Spec struct {
	Noun       string
	Short      string
	List       func(d *Deps, page, size int, sort []string) ([]byte, *client.APIError)
	ListFields []string
	Get        func(d *Deps, id int64, expand []string) ([]byte, *client.APIError)
	GetFields  []string
	Extra      []*cobra.Command // hand-written subcommands for non-standard ops
}

// Command builds the resource's cobra command tree.
func (s Spec) Command() *cobra.Command {
	parent := &cobra.Command{
		Use:   s.Noun,
		Short: s.Short,
		RunE:  UnknownSub,
	}
	if s.List != nil {
		parent.AddCommand(s.listCmd())
	}
	if s.Get != nil {
		parent.AddCommand(s.getCmd())
	}
	for _, e := range s.Extra {
		parent.AddCommand(e)
	}
	return parent
}

func (s Spec) listCmd() *cobra.Command {
	var page, size int
	var sort []string
	c := &cobra.Command{
		Use:   "list",
		Short: "List " + s.Noun + " (paginated)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := s.List(d, page, size, sort)
			if apiErr != nil {
				return RenderError(d.Printer, apiErr, "normatik "+s.Noun+" list")
			}
			d.Printer.Raw(body, s.ListFields...)
			return nil
		},
	}
	c.Flags().IntVar(&page, "page", 1, "page number (one-based)")
	c.Flags().IntVar(&size, "size", 10, "items per page")
	c.Flags().StringArrayVar(&sort, "sort", nil, "sort expression, e.g. name,asc (repeatable; server-side whitelist)")
	return c
}

func (s Spec) getCmd() *cobra.Command {
	var expand []string
	c := &cobra.Command{
		Use:   "get <id>",
		Short: "Get " + s.Noun + " detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := Build(cmd)
			if err != nil {
				return err
			}
			id, perr := ParseID(args[0])
			if perr != nil {
				d.Printer.Message("Error [USAGE]: <id> must be a number, got %q", args[0])
				return Handled(2)
			}
			body, apiErr := s.Get(d, id, expand)
			if apiErr != nil {
				return RenderError(d.Printer, apiErr, "normatik "+s.Noun+" get")
			}
			d.Printer.Raw(body, s.GetFields...)
			return nil
		},
	}
	c.Flags().StringSliceVar(&expand, "expand", nil, "expand sections (comma-separated)")
	return c
}
