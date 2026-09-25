# Third party code

- `go-mp3`: MP3 decoder (Apache 2.0), used to convert menu music.
- `rardecode`: RAR decoder by Nicholas Waples (BSD 2-Clause), v2.4.1 with one fix in `reader.go`:
  the first file of a solid archive is not marked solid, so its window must not be shrunk to its own
  size, or every later file in the archive decodes wrongly.
- `x-sync`, `x-text`, `go4`, `yaml3`: copies of Go modules from their GitHub mirrors, needed only to
  regenerate `vendor/`. They are not kept in the repo; run `fetch-mirrors.sh` to get them back.

The 7z reader is `github.com/bodgit/sevenzip` (BSD 3-Clause), vendored in `../vendor`.
