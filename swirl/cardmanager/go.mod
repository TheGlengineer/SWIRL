module swirlcardmanager

go 1.21

require github.com/hajimehoshi/go-mp3 v0.3.4

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/sevenzip v1.5.2
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/nwaples/rardecode/v2 v2.4.1
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	go4.org v0.0.0-20200411211856-f5505b9728dd // indirect
	golang.org/x/text v0.17.0 // indirect
)

replace github.com/hajimehoshi/go-mp3 => ./third_party/go-mp3

replace golang.org/x/sync => ./third_party/x-sync

replace golang.org/x/text => ./third_party/x-text

replace go4.org => ./third_party/go4

replace gopkg.in/yaml.v3 => ./third_party/yaml3

replace github.com/nwaples/rardecode/v2 => ./third_party/rardecode
