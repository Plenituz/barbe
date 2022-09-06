# Composing manifest: how to take full advantage of Barbe and pre-hash the work for your whole team

The manifest you use in a Barbe project is what determines which jsonnet templates will be used.
They can be versioned, inherit from other manifests, and even contain regular non-jsonnet files.
This guide explores all the different aspects of composing manifests.

```hcl
template {
  manifest = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
}
```

In this guide you will learn:
- [The different ways to use `template`]()
- [How to create your own manifest from scratch]()


## The different ways to use `template`

The `template` block in a Barbe configuration is where you indicate to Barbe what templates to use. To do so you have 3 options:
- Linking to a template file (or regular file) directly 
- Linking to a manifest file, which will contain a list of templates and can be versioned. Think of it as a base image if it was a Dockerfile
- Adding regular configuration files to the current execution of Barbe


### Linking to a template or config file directly

This is the simplest form of using templates. You list all the templates you want to use in your projects, and they will be executed together.
This however also removes a lot of the features that template developers might want to use, like versioning and order of execution.

```hcl
template {
  template = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/aws/aws_lambda.jsonnet"
  # or
  templates = [
    "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/aws/aws_lambda.jsonnet",
    "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/aws/aws_api_gateway.jsonnet"
  ]
}
```


In a similar vein, you can link to regular Barbe configuration file instead templates, these could host default value or resources that
you want every project in your company to have. This is the equivalent of copy-pasting the file in your local directly and running `barbe generate` on it.

```hcl
template { 
  file = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/examples/multi-region/config.hcl"
  # or
    files = [
        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/examples/multi-region/config.hcl",
        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/examples/api-gateway-nodejs/config.hcl"
    ]
}
```

### Linking to a manifest

You can have as many manifests as you want in your project, they give flexibility to the template developers to version
their releases, change the order of execution of templates, inherit from other manifests, and more.

As expected, this is the recommended way of using templates. 

```hcl
template {
  manifest = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
  # and/or
  manifest {
    url = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
    # optional, version selection for this specific manifest
    version = ">=0.1.0"
  }
  # and/or 
  manifest {
    url = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
  }
}
```

You can also mix and match all these options
```hcl
template {
  manifest = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
  file = "https://mycompany.com/default-values.hcl"
}
```

And you can define manifest/files locally
```hcl
template {
  manifest = "./my_manifest.json"
}
```

---

## How to create your own manifest from scratch

Now that we've seen how to use them, let's look inside and see how to create our own manifest.

### Manifest structure

Here is what the manifest for Barbe-serverless could look like
```json
{
    "latest": "0.0.2",
    "versions": {
        "0.0.1": {
            "steps": [
                {
                    "templates": [
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/c703a6bb/utils/for_each.jsonnet"
                    ]
                },
                {
                    "templates": [
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/c703a6bb/utils/passthrough.jsonnet",
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/c703a6bb/aws/aws_lambda.jsonnet"
                    ]
                }
            ]
        },
        "0.0.2": {
            "steps": [
                {
                    "templates": [
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/a9d08ed43/utils/for_each.jsonnet"
                    ]
                },
                {
                    "templates": [
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/a9d08ed43/utils/passthrough.jsonnet",
                        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/a9d08ed43/aws/aws_lambda.jsonnet"
                    ]
                }
            ]
        }
    }
}
```

There are 2 main parts to this manifest:
- The `latest` field, which indicates the latest version of the manifest
- The `versions` field, which contains a list of versions, each with a list of steps.

Think of each `steps` list as a data transformation pipeline, the parsed input file goes into the first step, gets transformed by the templates in that step, the transformed data then goes into the next step and so on.
The steps are pretty important as changing their order or the templates they contain could completely change the generated files.

In our example above the `for_each.jsonnet` template needs to be executed alone first as it could itself create new `aws_lambda` blocks.
If both the `for_each.jsonnet` template and the `aws_lambda.jsonnet` template were in the same step, the blocks that `for_each.jsonnet` creates wouldn't be seen by the `aws_lambda.jsonnet` template.

### Inheriting from other manifests

Using the `inheritFrom` field, you can define a list of manifests that will be used executed before the current one.
```json
{
    "latest": "0.0.1",
    "versions": {
        "0.0.1": {
            "inheritFrom": [
                {
                    "url": "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json",
                    "version": ">=0.0.1"
                }
            ],
            "steps": ["..."]
        }
    }
}
```

### Adding a message to your manifest

Defining a `message` field in your manifest version will display it to the user when they run `barbe generate`.
```json
{
    "latest": "0.0.1",
    "versions": {
        "0.0.1": {
            "message": "This version is deprecated and will stop working once AWS EOLs thing1",
            "steps": ["..."]
        }
    }
}
```


### Adding regular files

Similarly to template blocks, you can add regular files to your manifest.
```json
{
    "latest": "0.0.1",
    "versions": {
        "0.0.1": {
            "files": [
                "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/examples/multi-region/config.hcl"
            ],
            "steps": ["..."]
        }
    }
}
```


