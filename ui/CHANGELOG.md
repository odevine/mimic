# Changelog

## [0.6.0](https://github.com/odevine/mimic/compare/ui/v0.5.0...ui/v0.6.0) (2026-09-25)


### Features

* **ui:** render a pasted list to a folder, watched from a run console ([2a3292c](https://github.com/odevine/mimic/commit/2a3292c50221bbf1e214c24ccc24e3b6f6e15313))
* **ui:** resolve lists from a local copy of Scryfall's bulk data ([b3df74d](https://github.com/odevine/mimic/commit/b3df74d33c830edd9d4eed80d5520da919cafc92))


### Bug Fixes

* **ui:** pace Scryfall search and named lookups at two a second ([ce8d120](https://github.com/odevine/mimic/commit/ce8d120bcc7c55ee0fa098498709e6c25dbbccba))
* **ui:** refuse cross-site requests and pace Scryfall calls ([b8f603f](https://github.com/odevine/mimic/commit/b8f603ff514e12eb1e5cf9565c114c4df241801b))


### Build System

* **ui:** depend on the released engine v0.7.0 ([850911c](https://github.com/odevine/mimic/commit/850911c76f61eeb23b298abb3ae6700f6f64eee1))

## [0.5.0](https://github.com/odevine/mimic/compare/ui/v0.4.0...ui/v0.5.0) (2026-09-22)


### Features

* **ui:** rebuild the frontend as one shell with modes and gating ([17cf927](https://github.com/odevine/mimic/commit/17cf9270f1ff2d7dc6c8eb25ecb34dc27ef6df73))
* **ui:** serve feature gates, settings, printings and mana symbols ([6ff73fa](https://github.com/odevine/mimic/commit/6ff73fafc0e935d6440a313d5d9284ed1571dc83))

## [0.4.0](https://github.com/odevine/mimic/compare/ui/v0.3.0...ui/v0.4.0) (2026-09-22)


### Features

* **ui:** add preview and output resolutions ([fd38041](https://github.com/odevine/mimic/commit/fd380416ea55d7967f8676a758f86c1702196fc8))


### Build System

* **ui:** depend on the released engine v0.6.0 ([9d1448d](https://github.com/odevine/mimic/commit/9d1448daf4b8a3d42f747db744f335be439ef6f3))

## [0.3.0](https://github.com/odevine/mimic/compare/ui/v0.2.0...ui/v0.3.0) (2026-09-22)


### Features

* **ui:** show each template's description in the manager ([bfa98eb](https://github.com/odevine/mimic/commit/bfa98eb83b2ff648361148e161f3840a56dece38))


### Build System

* **ui:** depend on the released engine v0.5.0 ([f280e0b](https://github.com/odevine/mimic/commit/f280e0b59d1e718cfd60a56c4e6bc75eb408ff8c))


### Code Refactoring

* **engine:** pull template-agnostic logic out of normal ([b8dccb8](https://github.com/odevine/mimic/commit/b8dccb8f3b68d9531f3122c755bf38e031935a65))

## [0.2.0](https://github.com/odevine/mimic/compare/ui/v0.1.0...ui/v0.2.0) (2026-09-22)


### ⚠ BREAKING CHANGES

* **ui:** the ui is no longer a Fyne desktop app. It runs a local web server and opens the app in a browser, and the binary is renamed from mimic-ui to mimic.

### Features

* **ui:** replace Fyne desktop shell with a local web server ([030c615](https://github.com/odevine/mimic/commit/030c6158c114111506179067d38b9bd4f4a2a126))

## 0.1.0 (2026-09-22)


### Features

* **ui:** add a desktop app for search, edit, and preview ([fe0d28a](https://github.com/odevine/mimic/commit/fe0d28ae63531b51ba14f33b921199106634ef4b))
* **ui:** download bundles and select templates ([a1dc0e6](https://github.com/odevine/mimic/commit/a1dc0e6015a93d19a79a813390dde80d91de01bd))
* **ui:** show a render progress bar with step status ([c56e7e9](https://github.com/odevine/mimic/commit/c56e7e9983ede2a92d0ceb6c1f85983b212f9f3c))


### Miscellaneous Chores

* **ui:** release the first version as 0.1.0 ([4007567](https://github.com/odevine/mimic/commit/4007567b94c83eb3679caf55760e5f3b2bccb717))
