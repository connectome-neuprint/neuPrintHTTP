# llms.txt generator

neuPrintHTTP can serve an [llms.txt](https://llmstxt.org) file at `/llms.txt`
(see the `llms-txt` config field in the top-level README). This directory has
a small tool for producing that file.

- `llms.txt.j2`: Jinja template for neuprint.janelia.org. It mixes hand-written
  text with dataset descriptions taken from each dataset's `:Meta` node.
- `render_llms_txt.py`: renders the template, using dataset metadata from the
  server's public `GET /api/dbmeta/datasets` endpoint. No credentials are needed
  or used, so only publicly visible datasets can appear in the output.

## Usage

```bash
pip install jinja2 requests   # or: pixi exec -s python -s jinja2 -s requests -- python ...
python render_llms_txt.py llms.txt.j2 --server neuprint.janelia.org -o llms.txt
```

Review the output (hand-edit it if you like), then copy it to the path named by
`llms-txt` in the server config. The server re-reads the file on every request,
so the change takes effect immediately.

The script prints a note on stderr listing any public datasets the template
doesn't mention, as a reminder to decide whether to include them. If the template
calls `require("<dataset>")` for a dataset the server doesn't list (e.g. after a
version bump), rendering fails with a list of the available dataset names.

See the docstring at the top of `render_llms_txt.py` for the template variables.
