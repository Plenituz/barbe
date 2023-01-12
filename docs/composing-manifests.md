# Composing manifest: how to take full advantage of Barbe and pre-hash the work for your whole team

The manifest you use in a Barbe project is what determines which jsonnet templates will be used.
They can be versioned, import other manifests, and even contain regular non-jsonnet files with your company's defaults/preferences.
This guide explores all the different aspects of using manifests to accomplish more for your team.

```hcl
template {
  manifest = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
}
```

In this guide you will learn:
- [The different ways to use `template`](#the-different-ways-to-use-template)
- [How to create your own manifest from scratch](#how-to-create-your-own-manifest-from-scratch)


## The different ways to use `template`

The `template` block in a Barbe configuration is where you indicate to Barbe what templates to use. There are 3 kinds of "templates":
- Components: see [components](./components.md) for more details, these are the main characters in barbe, they define what blocks you can use. For example [`aws_http_api.jsonnet` on `barbe-serverless`](https://github.com/Plenituz/barbe-serverless/blob/main/aws/aws_http_api.jsonnet)
- Manifests: This is the root of most project, they are a collection of components, other manifests or configuration files. [barbe-serverless](https://github.com/Plenituz/barbe-serverless/blob/main/manifest.json) and [anyfront](https://github.com/Plenituz/anyfront/blob/main/manifest.json) both provide their own manifests for example. Think of it as a base image if it was a Dockerfile
- Configuration files: We call configuration file what is usually at the root of your repository and where the developer writes the infrastructure he wants to use. This is where you write your `gcp_next_js` or `aws_function` blocks for example. Manifests can also link to configuration files if some infrastructure is shared across all of your projects, like ci/cd definition


### Linking to components or config files directly

This is the simplest form of using the template block. You list all the components you want to use in your projects, and they will be executed together.

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

In a similar vein, you can link to regular configuration files, these could host default value or resources that
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

This is great if you're developing your own component and just want to try something out, but it quickly becomes inconvenient given the number of components usually involved in a project, this is where using manifests comes in.

### Linking to a manifest

You can have as many manifests as you want in your project, they help to keep consistent versioning across all the components you're using, and allow you to continuously update all the projects in company. You can remotely update the ci/cd definition, monitoring tools, etc by just updating the manifest.

```hcl
template {
  manifest = "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json"
  # or
  manifest = [
    "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json",
    "https://raw.githubusercontent.com/Plenituz/anyfront/main/manifest.json"
  ]
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

If you really want to take advantage of manifests, you'll want to define your own manifest where you add your own resources/components in addition to importing existing manifests. This is what allows you to have consistency across your company's projects, and save a lot of time to your teams.

If that doesn't sound like one of your needs using the ready-made manifests is great as well.

### Manifest structure

Here is what a manifest could look like
```json
{
    // If defined, this will be printed when the manifest is executed.
    // Can be used for deprecation notices for example
    "message": "You're using barbe-serverless v0.1.0",
    // This is the list of all the components that will be executed
    "components": [
        "https://company.com/custom_component.jsonnet",
    ],
    // This is the list of all the configuration files that will be added to the project
    "files": [
        "https://company.com/default-infrastructure.hcl"
    ],
    // This is the list of all the manifests that we're importing.
    // All the components and files defined in these will be executed before the current one
    "manifests": [
        "https://raw.githubusercontent.com/Plenituz/barbe-serverless/main/manifest.json",
        "https://raw.githubusercontent.com/Plenituz/anyfront/main/manifest.json"
    ],
}
```

In this example we're importing both the anyfront and barbe-serverless manifests, which lets us use all the blocks they already define seemlessly. But we're also adding our own custom component and a default infrastructure file, which we can update over time as our stack evolves.

You can imagine
- Switching from a Jenkins to a AWS Codebuild deployment pipeline without your developers having to know about it.
- Adding a new monitoring tool to all your projects without having to update them one by one.
- Including your custom service mesh service discovery registration directly in that `default-infrastructure.hcl` file.

