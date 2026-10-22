// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dict

import "io"

// IconProvider exposes a format's conventional dictionary icon, when the
// format defines one independently of its ordinary article resources.
type IconProvider interface {
	Icon() (io.ReadCloser, string, error)
}
