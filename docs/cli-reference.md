# Barbe CLI reference

```bash
barbe [command] [input files] [options]
```

## Commands

### `barbe generate`

`generate` creates all the files that will be used to deploy your project in the output directory (defaults to `barbe_dist`)

```bash
# Generate all the files needed to deploy the configuration in infra.hcl
barbe generate infra.hcl
```

### `barbe apply`

`apply` first runs `generate` and then deploys the generated files. This could mean many things depending on the configuration you're deploying: running `terraform apply`, running some AWS CLI commands, running some gcloud commands, etc

```bash
# Generate all the files needed to deploy the configuration in infra.hcl
# and then deploy them
barbe apply infra.hcl
```

### `barbe destroy`

`destroy` first runs `generate` and then tears down all the infrastucture previously. This could mean many things depending on the configuration you're deploying: running `terraform destroy`, running some AWS CLI commands, running some gcloud commands, etc

```bash
# Generate all the files needed to deploy the configuration in infra.hcl
# and then destroy them
barbe destroy infra.hcl
```

### `barbe version`

`version` prints the version of Barbe


## Options

### `-o, --output`

`output` specifies the output directory where the generated files will be stored. Defaults to `barbe_dist`

```bash
# Generated files will now be stored in the `dist` directory
barbe generate infra.hcl --output dist
barbe apply infra.hcl --output dist
barbe destroy infra.hcl --output dist
```

### `-e, --env`

`env` allows you to expose environment variables to the templates that are generating/deploying your infrastructure. By default Barbe exposes the following environment variables: `AWS_REGION`.

You can pass environment variables in 3 ways:
- `--env KEY=VALUE` will set the environment variable `KEY` to value `VALUE` for the templates
- `--env KEY` will expose the environment variable `KEY` to the templates, equivalent to `--end KEY=$KEY`
- `--env path/to/my/env/file` will set environment variables from the file `path/to/my/env/file`. The file should contain one environment variable per line, in the format `KEY=VALUE`


```bash
# Expose the environment variables `STAGE` and `DOMAIN` to the templates
barbe apply infra.hcl --env STAGE --env DOMAIN

# Set an explicit value for the environment variable `STAGE` and `DOMAIN` to the templates
barbe apply infra.hcl --env STAGE=prod --env DOMAIN=example.com

# Use a env file
barbe apply infra.hcl --env .production.env

# .production.env
STAGE=prod
DOMAIN=example.com
```

### `-l, --log-level`

`log-level` allows you to set the log level of Barbe, useful for debugging. Defaults to `info`. Possible values are: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`

```bash
# Set the log level to debug
barbe apply infra.hcl --log-level debug
```

### `--log-format`

`log-format` allows you to set the log format of Barbe. Defaults to `auto`. Possible values are: `plain`, `json`, `auto`

`auto` will use `plain` if the output is a terminal, and `json` if the output is not a terminal.

```bash
# Set the log format to json
barbe apply infra.hcl --log-format json
```

### `--no-input`

`no-input` will disable all the prompts that Barbe might ask you. This is useful for running Barbe in a CI environment.

```bash
# Disable all the prompts
barbe apply infra.hcl --no-input
```

### `--auto-approve`

`auto-approve` will automatically approve all the prompts that Barbe might ask you. This is useful for running Barbe in a CI environment.

```bash
# Automatically approve all the prompts
barbe apply infra.hcl --auto-approve
```

### `--debug-bags`

`debug-bags` will output the generated "databags" into `barbe_dist/debug-bags.json`. "databags" are the internal representation of the configuration that Barbe uses to generate and deploy your infrastructure. This is useful for debugging when creating components.

```bash
# Output the databags into `barbe_dist/debug-bags.json`
barbe apply infra.hcl --debug-bags
```