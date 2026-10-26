package homeassistantpkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

const haServiceMaintenanceReason = "Home Assistant service calls check maintenance for the selected server and all its known entities."

func (d *Deps) checkHAActionMaintenance(ctx context.Context, actionID, logicalTarget string, collector *hubcollector.Collector, entity string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if d.EvaluateAssetGuardrails == nil {
		return fmt.Errorf("Home Assistant maintenance evaluator unavailable")
	}
	lookup, ok := d.AssetStore.(persistence.HomeAssistantActionAssetStore)
	if !ok {
		return fmt.Errorf("Home Assistant action asset lookup unavailable")
	}
	check := func(ids []string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := maintenanceguard.CheckAssetActions(ids, d.EvaluateAssetGuardrails)
		var blocked *maintenanceguard.BlockedActionError
		if actionID == "service.call" && errors.As(err, &blocked) {
			blocked.PublicReason = haServiceMaintenanceReason
		}
		return err
	}
	if err := check([]string{collector.AssetID, logicalTarget}); err != nil {
		return err
	}
	if actionID == "service.call" {
		// Services can be global or expand area/device/label selectors. Keep
		// their payloads intact; conservatively guard all KNOWN entity owners.
		entity = ""
	}
	after := ""
	const pageSize = 128
	for page := 0; page < 32; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := lookup.ListHomeAssistantActionAssets(ctx, collector.ID, entity, after, pageSize)
		if err != nil {
			return err
		}
		if len(rows) > pageSize {
			return fmt.Errorf("Home Assistant asset lookup exceeded its page bound")
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if row.ID <= after {
				return fmt.Errorf("Home Assistant asset lookup did not advance")
			}
			if entity != "" && strings.TrimSpace(row.Metadata["entity_id"]) != entity {
				return fmt.Errorf("Home Assistant asset lookup returned a different entity")
			}
			if err := d.validateHAEntityOwner(row, collector.ID); err != nil {
				return err
			}
			if err := check([]string{row.ID}); err != nil {
				return err
			}
			after = row.ID
		}
		if len(rows) < pageSize {
			return ctx.Err()
		}
	}
	return fmt.Errorf("Home Assistant maintenance inventory exceeded its safety bound; action not sent")
}
