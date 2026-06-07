# Example

One canonical source, two runtimes.

```sh
loc build -c i18n/config.cue
loc stats -c i18n/config.cue
```

`app` reaches the web target only; `emails` reaches the backend only. The same
`{name}` arrives as `{name}` for vue-i18n and `{{.Name}}` for go-i18n.

The Polish `files` message carries all four CLDR bands plus the opt-in `zero`.
English carries two plus `zero`. Neither file mentions a pipe or a template
delimiter; the generator adds those per runtime.
