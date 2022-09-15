# Barbe 🧔

It takes more than 100 lines of terraform to configure autoscaling on a DynamoDB.
It takes 22 intricately connected resources to run a container image on AWS Fargate.

Barbe turns both of these into 6 lines. Simple to read, simple to write, as it should be.
```hcl
aws_dynamodb "my-table" {
  auto_scaling {
     min = 10
     max = 100
  }
}

aws_fargate_task "long-running-task" {
   docker {
      entrypoint = "./handler"
      runtime = "go"
   }
}
```

As developers, we care about concepts, ideas, like "serverless functions", "javascript bundling", "static website hosting".
Each vendor has their own way of implementing these concepts, and I don't want to learn 5 GCP services just to "deploy my Next.js front end".
Barbe does the heavy lifting of translating these concepts into vendor specific resources, so you can write `nextjs_hosting` instead of `google_cloud_run_service`, `google_container_registry` and 12 others.

### But it's more than that

Barbe is like the app store for configuration files. The docker hub for computer science concept implementations. 
You can publish your own definition of `nextjs_hosting`, or use ready-made definitions from the community.
For any concept you can imagine.

These definitions are written into templates that generate all the files needed to achieve what you asked for.

<p align="center">
  <img src="./readme_img_1.png" width="400" />
</p>

This approach has lot of advantages:
- Drastically reduce boilerplate of any toolchain
- Use a toolchain's best practices by default, forever
- Reduce cost of changing tooling and cloud platforms
- Easily glue together internal and public toolchains
- Gracefully handle deprecation of tooling features

Checkout [Barbe-serverless](https://github.com/Plenituz/barbe-serverless) to see how we use Barbe to make it super easy to use AWS
serverless resources (backed by Terraform)

> Barbe is in pretty early stage, be on the lookout for breaking changes, and come have fun with it!

## But what is it concretely?

Concretely, Barbe is a _programmable syntax manipulation engine_

It parses your configuration file as generic "syntax tokens", gives those syntax tokens to a number of templates that you chose, 
each one can manipulate and create more syntax tokens, Barbe then formats the result into files.

You specify the templates you want using a simple URL, kind of like a `FROM` statement in a Dockerfile.
This allows your configuration file to stay simple, but still harness a world a complexity that the template makers prepared for you.

Templates can also manipulate the syntax tokens it receives using dark magic, a simple reference like `cloudformation("my-stack").output.MyBucketName` in your Terraform file can be turned
into a concrete value gotten from your Cloudformation stack, without you lifting a finger.

#### An imaginary example project

Often when building cloud native applications, we have to glue together several toolchains, 
each of which can have its own configuration file, leading to having a number of files that depend on each other and 
potentially a lot of copy-pasting to do.

Let's imagine a project where we need:
 - A Terraform template with an AWS lambda function and all it's friends (logs, role, packaging, etc)
 - A JSON configuration file that our homemade service mesh reads to register the lambda function under an endpoint name `"action.do-something"`
 - A Webpack configuration to bundle our front end code

The config file below contains enough information for Barbe to generate all these files
```hcl
# config.hcl
template {
  # these are the links to the templates that will be applied to this config file
  manifest {
    # We can start from some open source template for the cloud resources
    url = "https://opensource-example.com/aws-serverless-stuff"
    # templates can be version constrained, or not
    version = ">=1.2.6"
  }
  manifest {
    # then you can add your own sprinkles, this would link to
    # the templates that generates our custom service mesh configs
    url = "https://mycompany.com/custom-service-mesh-stuff"
  }
}

# all of the properties of each block below are defined 
# and documented by the creators of the templates imported above
serverless_function "something-doer" {
    package_include = ["bin/something_doer"]
    service_mesh_endpoint = "action.do-something"
}

javascript_bundler "webpack" {
  typescript    = true
  static_assets = "./public/*"
  entry    = "src/App.tsx"
}
```

The templates defined in the `template` block abstract away the implementation details of using terraform with AWS, 
the format of our service mesh file, and the webpack.config.js. 

Also note how 2 completely separate templates (`custom-service-mesh-stuff` and `aws-serverless-stuff`) can pull from the same 
`serverless_function` block without stepping on each other's toes. 
This is one of the powerful aspect of Barbe allowing you to enhance your configuration file as the project evolves.

Of course sometimes you will want/have to use the configuration file for a specific tool directly (like the webpack config for example).
Templates can easily be designed to allow you to override parts of the generated configuration file, or even completely replace it.

> Note: In this example the Barbe config file is in [HCL](https://github.com/hashicorp/hcl), but Barbe is language agnostic

## How does it work?

To run barbe, you use the `generate` command: 
```bash
barbe generate config.hcl --output dist
```

The command works in 4 steps:

1. Parse the input file(s) `config.hcl` into an internal language agnostic syntax representation, "Databags" (basically collections of syntax tokens)
2. Download the templates defined in the `template` block. Templates are written
   in [Jsonnet](https://jsonnet.org/), a **_sandboxed_** declarative language
3. Evaluate each template giving it the parsed syntax tokens from step 1 as an input. Each template will generate
   additional syntax tokens
4. The syntax tokens will then be interpreted by various formatters which will generate the output files in the given "dist"
   directory

<p align="center">
  <img src="./readme_img_2.png" width="400" />
</p>

## Writing templates

If you're planning on writing templates (or are just curious) you can take a look at the [Writing Templates page](./docs/writing-templates.md) or [documentation](./docs/README.md)

You can also explore existing templates on the [Barbe-serverless](https://github.com/Plenituz/barbe-serverless) repository