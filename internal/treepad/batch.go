package treepad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/config"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
)

type BatchListInput struct {
	JSON      bool
	OutputDir string
}

// BatchList prints every Batch, its Chains, and each member's ticket, ref,
// branch and base. It creates no worktrees and calls gh for nothing.
func BatchList(ctx context.Context, d deps.Deps, in BatchListInput) error {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	commonDir, err := batch.CommonDir(ctx, d.Runner)
	if err != nil {
		return err
	}

	manifests, err := batch.Load(commonDir)
	if err != nil {
		return err
	}

	members := make([]batch.Member, 0)
	for _, m := range manifests {
		chains, err := batch.Resolve(m, cfg.FromSpec.TicketURL)
		if err != nil {
			return err
		}
		for _, chain := range chains {
			members = append(members, chain...)
		}
	}

	if in.JSON {
		return json.NewEncoder(d.Out).Encode(members)
	}
	if len(manifests) == 0 {
		d.Log.Info("no Batches found under %s", filepath.Join(commonDir, "treepad", "batches"))
		return nil
	}
	return writeBatchTable(d, members)
}

func writeBatchTable(d deps.Deps, members []batch.Member) error {
	for _, line := range formatBatchRows(members) {
		_, _ = fmt.Fprintln(d.Out, line)
	}
	return nil
}

func formatBatchRows(members []batch.Member) []string {
	if len(members) == 0 {
		return nil
	}
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BATCH\tCHAIN\tPOS\tTICKET\tREF\tBRANCH\tBASE")
	for _, m := range members {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			m.Batch, m.Chain, m.Position, m.Ticket, m.Ref, m.Branch, m.Base)
	}
	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}
