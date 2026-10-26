package internal

import (
	"context"
	"sort"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json: two routes for a workspace, two for an operator. The
// difference between them is exactly the `tenantEditable` check - an
// operator may override anything, a tenant only what the platform allows.
type api struct {
	repo  repository
	write writer
	defs  []flags.Definition // sorted by identifier
	byID  map[string]flags.Definition
}

var _ StrictServerInterface = api{}

func (a api) GetSettingsIndex(ctx context.Context, _ GetSettingsIndexRequestObject) (GetSettingsIndexResponseObject, error) {
	if _, err := auth.Require(ctx, "settings.view"); err != nil {
		return nil, err
	}
	overrides, err := a.repo.forTenant(ctx)
	if err != nil {
		return nil, err
	}
	out := TenantSettingList{Items: make([]TenantSetting, 0, len(a.defs))}
	for _, d := range a.defs {
		s := settingView(d)
		item := TenantSetting{Id: s.Id, Identifier: s.Identifier, Name: s.Name, Description: s.Description, DefaultValue: s.DefaultValue,
			TenantEditable: s.TenantEditable, Category: s.Category, Version: s.Version, EffectiveValue: d.Default}
		item.Type = TenantSettingType(s.Type)
		if v, ok := overrides[d.Identifier]; ok {
			item.EffectiveValue, item.Overridden = v, true
		}
		out.Items = append(out.Items, item)
	}
	return GetSettingsIndex200JSONResponse(out), nil
}

func (a api) PutSettingsSet(ctx context.Context, req PutSettingsSetRequestObject) (PutSettingsSetResponseObject, error) {
	p, err := auth.Require(ctx, "settings.manage")
	if err != nil {
		return nil, err
	}
	d, ok := a.byID[req.Identifier]
	if !ok {
		return nil, httpx.NotFound(keyUnknown)
	}
	if !d.TenantEditable {
		// A tenant re-enabling what an operator switched off during an
		// incident would defeat the point of switching it off.
		return nil, httpx.BadRequest(keyPlatformManaged)
	}
	o, err := a.write.own(ctx, p.TenantID, req.Identifier, req.Body.Value)
	if err != nil {
		return nil, err
	}
	return PutSettingsSet200JSONResponse(o.view()), nil
}

func (a api) GetManagerSettingsIndex(ctx context.Context, _ GetManagerSettingsIndexRequestObject) (GetManagerSettingsIndexResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	out := SettingList{Items: make([]Setting, 0, len(a.defs))}
	for _, d := range a.defs {
		out.Items = append(out.Items, settingView(d))
	}
	return GetManagerSettingsIndex200JSONResponse(out), nil
}

func (a api) PutManagerSettingsSet(ctx context.Context, req PutManagerSettingsSetRequestObject) (PutManagerSettingsSetResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	if _, ok := a.byID[req.Identifier]; !ok {
		return nil, httpx.NotFound(keyUnknown)
	}
	o, err := a.write.forTenant(ctx, req.TenantId, req.Identifier, req.Body.Value)
	if err != nil {
		return nil, err
	}
	return PutManagerSettingsSet200JSONResponse(o.view()), nil
}

func catalogue(defs []flags.Definition) ([]flags.Definition, map[string]flags.Definition) {
	sorted := append([]flags.Definition(nil), defs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Identifier < sorted[j].Identifier })
	byID := make(map[string]flags.Definition, len(sorted))
	for _, d := range sorted {
		byID[d.Identifier] = d
	}
	return sorted, byID
}
