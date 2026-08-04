# a GitHub CLI fork

This is a fork of GitHub CLI, which brings GitHub pull requests, issues, Actions,
and other workflows to the command line. See the
[upstream repository](https://github.com/cli/cli) for full documentation.

## about this fork

This fork carries the upstream CLI with one user-facing addition:

- multi-column job layouts for `gh run watch`.

Apart from this addition, the fork behaves like the upstream revision on which
it is based.

### multi-column run watching

Use `--columns` when a workflow has enough jobs to outgrow the height of the
terminal:

```shell
# Request up to three columns
gh run watch --columns 3

# Choose a column count from the terminal width
gh run watch --columns auto
```

The default is `--columns 1`, which preserves the upstream single-column
layout. An explicit positive number requests up to that many columns;
`--columns auto` chooses up to four columns, aiming to leave about 40 characters
for each one. Both forms work with the full and `--compact` displays.

Jobs fill each column from top to bottom while the layout balances their
heights. A job and its steps are never split between columns. If there are fewer
jobs than requested columns, or the terminal cannot give every column at least
24 characters, fewer columns are used. Lines too wide for a column are
truncated rather than wrapped.

## building

Building requires Go 1.26 or later. On Unix-like systems:

```shell
make
```

The resulting binary is `bin/gh`. See the upstream
[source installation guide](docs/install_source.md) for installation and
Windows instructions.

## license

GitHub CLI and this fork are released under the [MIT License](LICENSE).
