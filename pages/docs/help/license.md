---
title: Licence
description: WuWeiDict is free software under the GNU GPL, version 3 or later; the third-party code it includes.
---

# Licence

**WuWeiDict** is free software. You may redistribute it and change it under the
terms of the **GNU General Public License**, as published by the Free Software
Foundation, either version 3 of the license or, at your option, any later
version.

This program is distributed in the hope that it is useful, but **with no
warranty**, and without even the implied warranty of merchantability or fitness
for a particular purpose.

The full text is in the repository at `LICENSE`, and at the
[Free Software Foundation](https://www.gnu.org/licenses/gpl-3.0.html).
`wudict licenses` prints it with the notices for all third-party code.

## Third-party code

| Component | Origin | Licence |
| --- | --- | --- |
| MDX and MDD parser (`internal/gomdict`) | [medict](https://github.com/terasum/medict) (formerly go-mdict), © 2023 Quan Chen | GPL-3.0-or-later |
| BGL parser (`internal/format/bgl`) | ported from [pyglossary](https://github.com/ilius/pyglossary)'s `babylon_bgl` plugin; streaming modelled on [goldendict-ng](https://github.com/xiaoyifang/goldendict-ng) | GPL-3.0-or-later |
| Speex decoder (`internal/speex/clib`) | [Speex](https://www.speex.org/), © Jean-Marc Valin, Xiph.Org, Analog Devices | BSD-3-Clause |
| `kiss_fft` (`internal/speex/clib`) | [kissfft](https://github.com/mborgerding/kissfft), © Mark Borgerding | BSD-3-Clause |

The BGL format was reverse-engineered by Raul Fernandes and Karl Grill. The
notices for all dependencies are in
[THIRD-PARTY-NOTICES.md](https://github.com/wuweidict/wudict/blob/master/THIRD-PARTY-NOTICES.md).
