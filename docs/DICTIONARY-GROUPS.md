# Dictionary groups

Open **☰ → Settings → Edit dictionary groups**. The selector lists
**All Dictionaries**, your groups, and **New Group** last. New groups are empty.
Turn on **Show All** to add dictionaries using the checkboxes; turn it off to
see only members. Members appear first, followed by nonmembers, each in the
existing dictionary order. A dictionary can belong to several groups.

While editing membership with Show All enabled, a Filter dropdown separates
current members from dictionaries available to add. It filters only the latter,
using upstream language, language-pair and rules-based classifications. All
dictionaries is the default; Uncategorized means no inferred classifications.
Current members stay visible, and adding a dictionary moves it above the filter.
Empty categories are omitted, including Uncategorized. If adding a dictionary
empties the selected category, the filter returns to All dictionaries.

**All Dictionaries** always includes the current collection, including newly
added dictionaries. Its checkboxes cannot be cleared. Removing a dictionary
from a user group does not remove its files; it only changes which group
selects it. This editor does not change search filters; renaming and deleting
groups are outside its scope.

## Storage and API

Groups are additive fields in the existing version 1 `state.json`, beside the
active configuration. `groups` contains `{id, name}` records; each dictionary
preference may contain `groups`, an array of group IDs. Older files need no
rewrite until the next save. Existing order, enabled state and UI preferences
are retained. Existing dictionary ID/path healing also carries membership when
a dictionary moves. Unavailable dictionaries are hidden but their membership
is retained, like their other preferences, so unplugging a drive loses nothing.
All Dictionaries is computed from the registry and is never stored as a list.

Private, authenticated, same-origin endpoints:

- `GET /api/user-groups`: array of `{id, name, members, readonly}`.
- `POST /api/user-groups`: `{name}` creates an empty group and returns it.
- `PUT /api/user-groups/member`: `{group, dict, member}` changes one membership.

Names are trimmed, limited to 100 characters, and checked case-insensitively
against existing names and the reserved All Dictionaries name. Empty names and
control characters are rejected. Saves serialize read/merge/write operations
and go through `state.json`'s owned file (`ownedfile.go`): written atomically,
and a failed save leaves the file — and the value in effect — as they were.
Membership never writes the search `off` flag.

### Two kinds of "groups"

Upstream's own groups feature lives beside this one and is a different thing:
`groups.ini`, the RULES that derive a group from a dictionary's own name, path
and language (`internal/facet/rules.go`), edited through `GET/PUT/DELETE
/api/groups` and the `/groups` page. It computes; this editor curates. A
dictionary can have both classifications in storage, but the fork's picker
uses only curated membership. The CLI selects this behavior with
`Server.UseUserGroups()` at startup; upstream facets are returned separately as
`filters` in `/api/dicts` for the membership editor, while `groups` and upstream
diagnostics stay out of the fork's search picker. Article-language detection is retained.

The two are deliberately kept apart so upstream's half can be taken verbatim on
every sync: its API keeps the `/api/groups` path and its file keeps the
`groups.go` name, while this one lives in `dmsh_groups.go` (the fork's `dmsh_`
prefix for a parallel implementation) behind `/api/user-groups`. Settings
links to the upstream rules editor at `/groups` as **Edit filters**. Its EN/RU
interface edits the same rules file and retains the upstream API.

## Verification

`go test ./internal/server -run 'TestGroup|TestPrefs|TestOpenAPI|TestRoutes|TestCORSBoundary'`
covers persistence, overlapping membership, automatic collection changes,
validation, failed saves, identity healing and concurrent preference saves.
Run the repository's `make check` on its supported build environment as well.

For manual UI verification, create Essential and Full, enable Show All and add
the same dictionary to both. Turn Show All off, remove a member, reopen the
editor and restart the server. Check that All Dictionaries remains complete,
that an empty group offers the Show All hint, and that Cancel creates nothing.
Check light/dark and custom paper backgrounds on a narrow viewport.
