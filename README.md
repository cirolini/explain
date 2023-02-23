# Explain

`Explain` is a linux program that provides ChatGPT explanation for Linux terminal commands.

## Table of contents

- [General Requirements](#general-requirements)
- [Usage](#usage)
- [Using Docker](#run-the-program-in-a-docker-container)
- [Troubleshooting](#troubleshooting)


## General Requirements
- You must have an _API key_, which you can get from [this site](https://platform.openai.com/account/api-keys).


## Usage
- Create a file named `.explainrc` in your home directory

```bash
touch ~/.explainrc
```

- Paste your retrieved _Open AI API key_ inside the file you just created

```bash
echo {YOUR_KEY} > .explainrc
```

- Download the executable from [here](https://github.com/cirolini/explain/releases/tag/1.0.0), accordingly to your Linux architecture.
- You may rename the archive to one that suits you better, for example, "explain".

```bash
mv linux-amd64 explain
```

- Modify its permissions to make it possible to be called.

```bash
chmod +x explain
```

- Copy the executable to your `usr/local/bin` directory, so you can invoke it directly by its name.

```bash
cp explain /usr/local/bin
```

- Finally, now you just need to call it passing some args, and wait for the response.
- Your Args are one or more Linux terminal commands, which you must provide sepparated only by a white space.
- The greater the number of args are, the program will take longer to respond. If you provide three args or less, it shouldn't take more than thirty seconds.


```bash
explain ls sudo grep
```

- Have in mind that the time of response may oscilate, depending on the ChatGPT server latency and the number of requests being made. Also, if you make various requests in a small amount of time, your api key may be invalidated. See [Troubleshooting](#troubleshooting)

## Run the program in a docker container 
- Get the docker image from [Docker hub](https://hub.docker.com/r/adrancarnavale/explain) (ensure you get the one tagged as `latest`)
- Run is as follows:

```bash
docker run -e PROMPT="YOUR_PROMPT" -e API_KEY="YOUR_API_KEY" adrancarnavale/explain:latest
```

- Your prompt is made of one or more Linux terminal commands, which you must provide sepparated only by a white space.
- Your API key is the one which you retrieved from the site indicated above

### Example

```bash
docker run -e PROMPT="lsof sudo grep" -e API_KEY="sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" adrancarnavale/explain:latest
```

## Troubleshooting

- If you api key has been revoked, you may retrieve another one from [here](https://platform.openai.com/account/api-keys), and substitute the older one in your local files, or in your docker container.

