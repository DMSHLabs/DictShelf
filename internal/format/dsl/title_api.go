// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

// HeadwordKeys applies the DSL title rules to one title line and returns its
// searchable forms. Binary containers that store DSL title syntax can use the
// same optional-part, unsorted-part and escaping rules as text DSL files.
func HeadwordKeys(line string) []string { return transformTitle(line).Keys }

// HeadwordDisplay returns the HTML display title produced by DSL unsorted
// parts, alongside the searchable keys. display is empty for ordinary titles.
func HeadwordDisplay(line string) string { return transformTitle(line).Display }
