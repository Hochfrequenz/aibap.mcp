package bwf

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// ErrNotFound is returned when no workflow definition exists for a task.
var ErrNotFound = errors.New("workflow definition not found")

var versionRe = regexp.MustCompile(`^\d{4}$`)

type versionInfo struct {
	WfdID, Version, Source, Tables string // Tables: "SWDS" | "SWD"
	RuntimeWfdID, RuntimeVersion   string
	HasConfiguration               bool
	Warnings                       []string
}

const transportVersion = "9999"

// resolveVersion applies the "Active version rule" of the design.
func resolveVersion(ctx context.Context, q adt.QueryClient, task, requested string, today time.Time) (versionInfo, error) {
	var info versionInfo
	if !taskRe.MatchString(task) {
		return info, fmt.Errorf("invalid workflow task %q", task)
	}
	if requested != "" && !versionRe.MatchString(requested) {
		return info, fmt.Errorf("invalid version %q", requested)
	}
	objid := strings.TrimPrefix(task, "WS")

	hdr, err := fetch(ctx, q, fmt.Sprintf(
		"SELECT wfd_id, version, language, activat_on FROM swdsheader WHERE swdsheader~task = '%s'", task))
	if err != nil {
		return info, err
	}
	hdr = filterTask(hdr, task)

	if len(hdr) > 0 {
		info.Tables = "SWDS"
		if err := chooseSWDS(ctx, q, &info, hdr, task, objid, requested, today); err != nil {
			return info, err
		}
	} else if err := chooseLegacy(ctx, q, &info, task, requested); err != nil {
		return info, err
	}

	info.RuntimeWfdID, info.RuntimeVersion, info.HasConfiguration, err = resolveRuntime(ctx, q, task)
	return info, err
}

func chooseLegacy(ctx context.Context, q adt.QueryClient, info *versionInfo, task, requested string) error {
	legacy, err := fetch(ctx, q, fmt.Sprintf(
		"SELECT wfd_id, version, exetyp FROM swd_header WHERE swd_header~task = '%s' AND exetyp = 'S'", task))
	if err != nil {
		return err
	}
	legacy = keepEq(filterTask(legacy, task), "EXETYP", "S")
	if len(legacy) == 0 {
		return fmt.Errorf("%w: no definition for %s in SWDSHEADER, SWD_HEADER", ErrNotFound, task)
	}
	info.Tables = "SWD"
	info.Source = "legacy_client"
	if requested != "" {
		if len(keepEq(legacy, "VERSION", requested)) == 0 {
			return fmt.Errorf("version %s of %s not found (available: %s)", requested, task, versionList(legacy))
		}
		info.Source = "requested"
		info.Version = requested
	} else {
		info.Version = highestVersion(legacy, false)
	}
	var ids []string
	for _, r := range legacy {
		if r["VERSION"] == info.Version {
			ids = append(ids, r["WFD_ID"])
		}
	}
	sort.Strings(ids)
	info.WfdID = ids[len(ids)-1]
	if len(ids) > 1 {
		info.Warnings = append(info.Warnings, fmt.Sprintf(
			"SWD_HEADER has several legacy definitions of %s version %s (WFD_IDs %s); using the highest, %s",
			task, info.Version, strings.Join(ids, ", "), info.WfdID))
	}
	return nil
}

func chooseSWDS(ctx context.Context, q adt.QueryClient, info *versionInfo, hdr []Row, task, objid, requested string, today time.Time) error {
	if requested != "" {
		info.Source = "requested"
		info.Version = requested
	} else if err := activePointer(ctx, q, info, objid, today); err != nil {
		return err
	}
	if info.Version == "" {
		info.Source = "latest_numbered"
		info.Version = highestVersion(hdr, true)
		info.Warnings = append(info.Warnings, "no activation pointer (HRS1205/HRP1205) found; using the latest numbered version")
	}
	if info.Version == "" {
		return fmt.Errorf("%w: %s has only the transport version %s in SWDSHEADER", ErrNotFound, task, transportVersion)
	}
	for _, r := range hdr {
		if r["VERSION"] == info.Version {
			info.WfdID = r["WFD_ID"]
			return nil
		}
	}
	return fmt.Errorf("version %s of %s not found in SWDSHEADER (available: %s)", info.Version, task, versionList(hdr))
}

// activePointer fills info.Version/Source from HRS1205, else HRP1205. It leaves
// them empty if neither table has an active pointer. Filtering is repeated in Go
// so the rule holds even if the SQL filtered nothing.
func activePointer(ctx context.Context, q adt.QueryClient, info *versionInfo, objid string, today time.Time) error {
	rows, err := fetch(ctx, q, fmt.Sprintf(
		"SELECT objid, wfd_id, version, active FROM hrs1205 WHERE otype = 'WS' AND objid = '%s' AND active = 'X'", objid))
	if err != nil {
		return err
	}
	rows = keepEq(keepEq(rows, "OBJID", objid), "ACTIVE", "X")
	src := "HRS1205"
	if len(rows) == 0 {
		rows, err = fetch(ctx, q, fmt.Sprintf(
			"SELECT objid, wfd_id, version, active, begda, endda FROM hrp1205 WHERE otype = 'WS' AND objid = '%s' AND active = 'X'", objid))
		if err != nil {
			return err
		}
		d := today.Format("20060102")
		var valid []Row
		for _, r := range keepEq(keepEq(rows, "OBJID", objid), "ACTIVE", "X") {
			if r["BEGDA"] <= d && d <= r["ENDDA"] {
				valid = append(valid, r)
			}
		}
		rows = valid
		src = "HRP1205"
	}
	if len(rows) == 0 {
		return nil
	}
	info.Source = src
	info.Version = highestVersion(rows, false)
	if len(rows) > 1 {
		info.Warnings = append(info.Warnings, fmt.Sprintf(
			"%s has several active rows (versions %s); using the highest, %s", src, versionList(rows), info.Version))
	}
	return nil
}

// resolveRuntime reads the runtime copy and configuration flag from SWD_HEADER.
func resolveRuntime(ctx context.Context, q adt.QueryClient, task string) (wfdID, version string, hasConfig bool, err error) {
	if !taskRe.MatchString(task) {
		return "", "", false, fmt.Errorf("invalid workflow task %q", task)
	}
	rows, err := fetch(ctx, q, fmt.Sprintf("SELECT wfd_id, version, exetyp FROM swd_header WHERE swd_header~task = '%s'", task))
	if err != nil {
		return "", "", false, err
	}
	for _, r := range filterTask(rows, task) {
		switch r["EXETYP"] {
		case "E":
			if r["VERSION"] >= version {
				version, wfdID = r["VERSION"], r["WFD_ID"]
			}
		case "C", "A":
			hasConfig = true
		}
	}
	return wfdID, version, hasConfig, nil
}

// filterTask drops rows that carry a TASK column with a different value.
func filterTask(rows []Row, task string) []Row {
	var out []Row
	for _, r := range rows {
		if t, ok := r["TASK"]; ok && t != task {
			continue
		}
		out = append(out, r)
	}
	return out
}

// keepEq keeps rows whose col equals val (rows lacking the column are kept).
func keepEq(rows []Row, col, val string) []Row {
	var out []Row
	for _, r := range rows {
		if v, ok := r[col]; !ok || v == val {
			out = append(out, r)
		}
	}
	return out
}

// highestVersion returns the maximum VERSION; skipTransport ignores 9999.
func highestVersion(rows []Row, skipTransport bool) string {
	best := ""
	for _, r := range rows {
		v := r["VERSION"]
		if skipTransport && v == transportVersion {
			continue
		}
		if v > best {
			best = v
		}
	}
	return best
}

func versionList(rows []Row) string {
	seen := map[string]bool{}
	var vs []string
	for _, r := range rows {
		if v := r["VERSION"]; !seen[v] {
			seen[v] = true
			vs = append(vs, v)
		}
	}
	sort.Strings(vs)
	return strings.Join(vs, ", ")
}
