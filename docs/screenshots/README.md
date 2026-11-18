# Screenshots

The screenshots in the [README](../../README.md) are generated with [VHS](https://github.com/charmbracelet/vhs)
from the tapes in this directory. With the dev cluster up and filled (`make up && make generate`), run

```sh
make screenshots
```

or `./docs/screenshots/screenshots.sh` without `make`.

It builds ktea, runs it with an isolated configuration pointing at the dev cluster (see `config/`, your own
configuration is not used) and overwrites the images in this directory. VHS is taken from the `PATH`, or from
nixpkgs when [nix](https://nixos.org) is installed.

| File | Purpose |
|------|---------|
| `setup.tape` | shared settings (size, font, theme) and starting ktea, sourced by the other tapes |
| `topics-page.tape` | the topics of the dev cluster |
| `records-page.tape` | the records of a topic with the preview of the selected record |
| `schema-page.tape` | an Avro schema in the schema registry |
| `consumer-lag.tape` | the offsets and lag of the `generator` consumer group |
| `config/` | the ktea configuration used for the screenshots |

The offsets and counts depend on the generated data, so they change slightly with every run.
