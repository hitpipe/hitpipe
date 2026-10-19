# HIT-53 correction CLI evidence

The correction head `df1a7d58179148abfda8e71862654f5ef7fcdbc7` was executed by
the owned GitHub Actions workflow at
`https://github.com/hitpipe/hitpipe/actions/runs/39274386817` (job
`118134753907`, `ubuntu-24.04`, Go `go1.27.2`). The workflow's declared smoke
route runs the diagnostic binary in both dev and release outside-checkout
distributions and exercises:

```text
--fixture <positive-fixture> --format json       -> exit 0, verified output
--fixture <negative-fixture> --format json      -> non-zero rejection
--fixture <positive-fixture> --format invalid   -> non-zero bounded error
```

The existing CLI implementation maps the last case to safe reason
`format_invalid` and exit `2`; `text` and `json` are the two valid output
representations. The CI conclusion was `success`. No runtime configuration flag
was invented and no live provider data was used.
