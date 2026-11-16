# bowel Packaging Exception

This is an additional permission from Oboo Cheng (the licensor) on top of the [PolyForm Noncommercial License 1.0.0](LICENSE) that bowel is licensed under. It adds to that license and takes nothing away from it.

## Permission

Anyone, whether or not they are a business, may:

1. build bowel from its source code;
2. make the changes needed to package it for an operating system, distribution or app store, and to make it work there;
3. distribute the resulting packages through any package repository, app store or download site.

This covers, among others, Flathub, the Snap Store, the Arch Linux repositories and the AUR, Debian and Ubuntu (non-free), RPM Fusion (nonfree) and openSUSE (non-oss), and the repositories of commercial distributions.

## Conditions

1. **Notices.** Each package includes `LICENSE`, with its `Required Notice:` line, and this file. Its metadata gives the license as `PolyForm-Noncommercial-1.0.0` with this exception, or marks bowel as non-free or proprietary, whichever the repository or store uses.
2. **No sale.** No fee is charged for bowel itself, and bowel is not sold or offered as a feature of a paid product or service. A fee or subscription for a whole distribution, repository or store is fine, as long as it is the same whether or not bowel is included.
3. **Packaging changes only.** Changes are limited to build settings, file locations, desktop and system integration, dependencies, and fixes needed to build or run bowel on the target system. A package does not remove or hide the license notices, and is not presented as released by the licensor. The name bowel and its icon may be used to identify the package.

## What this does not cover

- **Use.** People who install a package get bowel under the PolyForm Noncommercial License 1.0.0, and only for noncommercial purposes. This exception gives them no right to use bowel commercially. For a commercial license, [open an issue](https://github.com/21stware/bowel/issues).
- **Other changes and new works.** Changes beyond packaging, and works based on bowel, fall under the PolyForm Noncommercial License 1.0.0 alone.
- **Third-party parts.** The bundled mihomo core is under GPL-3.0, and a package that includes it has to meet that license, for example by offering the matching source. The flag images in `assets/flags/` and the fonts in `resources/fonts/` keep their own licenses.

## Versions

The licensor may publish a new version of this exception. A release of bowel stays covered by the version of this file it shipped with.
