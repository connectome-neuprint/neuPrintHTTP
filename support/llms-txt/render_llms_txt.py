#!/usr/bin/env python3
"""
Render an llms.txt file for a neuPrint server from a Jinja template.

The template can embed dataset metadata (the :Meta node properties) obtained
from the server's public GET /api/dbmeta/datasets endpoint.  The request is
made anonymously, so only datasets which are publicly visible can be rendered.

The output is an ordinary text file.  Review (or hand-edit) it, then copy it
to the location named by the "llms-txt" key in the neuPrintHTTP config.
The server re-reads the file on every request, so no restart is needed.

Template variables:

    datasets:
        dict of {dataset_name: metadata}, as returned by /api/dbmeta/datasets.
        Each metadata entry has keys such as 'description', 'info', 'last-mod',
        'logo', 'ROIs', 'superLevelROIs', 'uuid', and 'hidden'.
        Use this for optional datasets, e.g.:

            {% if "hemibrain:v1.2.1" in datasets %}
            {{ datasets["hemibrain:v1.2.1"].description }}
            {% endif %}

    require(name):
        Returns the metadata for the named dataset,
        or aborts rendering if the server doesn't list it (e.g. after a rename).

    server:
        The server URL, e.g. "https://neuprint.janelia.org".

    today:
        Today's date (YYYY-MM-DD).

After rendering, any public datasets on the server which the template never
mentions are listed on stderr, as a reminder to decide whether to include them.

Usage:

    python render_llms_txt.py llms.txt.j2 --server neuprint.janelia.org -o llms.txt

Requirements: jinja2, requests
"""
import argparse
import datetime
import sys
from pathlib import Path

import jinja2
import requests


class _TrackedDatasets(dict):
    """
    A dict which records which dataset names the template looked up,
    so we can report the datasets the template never mentions.
    """
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.mentioned = set()

    def __getitem__(self, key):
        self.mentioned.add(key)
        return super().__getitem__(key)

    def __contains__(self, key):
        self.mentioned.add(key)
        return super().__contains__(key)

    def get(self, key, default=None):
        self.mentioned.add(key)
        return super().get(key, default)


def fetch_datasets(server):
    r = requests.get(f"{server}/api/dbmeta/datasets", timeout=60)
    r.raise_for_status()
    return r.json()


def render(template_path, server, datasets):
    datasets = _TrackedDatasets(datasets)

    def require(name):
        if not dict.__contains__(datasets, name):
            raise RuntimeError(
                f"Template requires dataset '{name}', but {server} doesn't list it. "
                f"Available datasets: {', '.join(sorted(datasets))}"
            )
        return datasets[name]

    template_path = Path(template_path)
    env = jinja2.Environment(
        loader=jinja2.FileSystemLoader(template_path.parent),
        undefined=jinja2.StrictUndefined,
        keep_trailing_newline=True,
        trim_blocks=True,
        lstrip_blocks=True,
    )
    template = env.get_template(template_path.name)
    text = template.render(
        datasets=datasets,
        require=require,
        server=server,
        today=datetime.date.today().isoformat(),
    )
    unmentioned = sorted(set(datasets) - datasets.mentioned)
    return text, unmentioned


def main():
    parser = argparse.ArgumentParser(description=__doc__.split('\n\n')[0])
    parser.add_argument('template', help='Path to the Jinja template, e.g. llms.txt.j2')
    parser.add_argument('--server', default='neuprint.janelia.org',
                        help='neuPrint server to read dataset metadata from (default: %(default)s)')
    parser.add_argument('-o', '--output', help='Output file (default: stdout)')
    args = parser.parse_args()

    server = args.server.rstrip('/')
    if not server.startswith(('http://', 'https://')):
        server = f"https://{server}"

    text, unmentioned = render(args.template, server, fetch_datasets(server))

    if args.output:
        with open(args.output, 'w') as f:
            f.write(text)
    else:
        sys.stdout.write(text)

    if unmentioned:
        print(f"Note: the template doesn't mention these public datasets: {', '.join(unmentioned)}",
              file=sys.stderr)


if __name__ == '__main__':
    main()
