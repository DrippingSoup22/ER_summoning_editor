# Third-party notices

## Save and regulation implementation

The save container, the regulation and its row insertion and removal are
[ER_shared](https://github.com/DrippingSoup22/ER_shared), the same author's
library, which descends from ER Merchant Editor's save code and earlier work
derived from [EldenRing-SaveForge](https://github.com/oisis/EldenRing-SaveForge).
All are licensed GPL-3.0-or-later, as this product is. `internal/savecore`
holds only this product's patch families on top of it.

`reference/` holds this project's own research outputs. The evidence behind
them remains in the private research repository rather than being copied into
this product.

## Desktop editor dependencies

The GUI links Gio (`gioui.org`), dual-licensed under the UNLICENSE or MIT, and
`github.com/ncruces/zenity` under the MIT license, together with their own
dependencies as recorded in `go.mod`. The command-line `ersummoneditor` links
none of them.

`internal/ui/gio/assets/fonts/` vendors the Cinzel and Lora typefaces under the
SIL Open Font License, whose full text is kept beside each font file as
`Cinzel-OFL.txt` and `Lora-OFL.txt`.

`internal/platform/dialogs` is ported from ER Merchant Editor, the same
author's project, which is licensed GPL-3.0-or-later as this one is.

`internal/ui/gio/assets/ashes/` holds the 84 Spirit Ash item icons, one per
family, copied from that same project's vendored icon set. It in turn vendored
them from the GPLv3-licensed
[EldenRing-SaveForge](https://github.com/oisis/EldenRing-SaveForge) project;
they are game art and remain the property of their respective owners, included
here only so that an Ash is shown to a player as the picture they would
recognise from their own inventory. They are embedded under `internal/ui/gio`
rather than in a shared package so that the command-line binary, which draws
nothing, does not carry them.

`internal/ui/gio/assets/items/` holds the icons of the items the character
builder offers, and `internal/items/items.csv` their names and categories, both
generated from the same project's vendored item set, with the
same provenance and the same reason as the Ash icons.

Elden Ring names and game data remain the property of their respective owners.
They are included only as the minimal factual catalog required for this editor.

## Creature pictures

`internal/ui/gio/assets/entities/` holds the creature pictures the catalog
shows, made for this project with ChatGPT's image generation.
