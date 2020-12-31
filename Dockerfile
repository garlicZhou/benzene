FROM ubuntu:18.04

SHELL ["/bin/bash", "-c"]

RUN apt update && apt upgrade -y

ENV GOPATH=/root/go
ENV GO111MODULE=on
ENV HMY_PATH=${GOPATH}/src/github.com/harmony-one
ENV GIMME_GO_VERSION="1.14.12"
ENV PATH="/root/bin:${PATH}"

RUN apt-get update -y
RUN apt install libgmp-dev libssl-dev curl git \
psmisc dnsutils jq make gcc g++ bash tig tree sudo vim \
silversearcher-ag unzip emacs-nox nano bash-completion -y

RUN mkdir ~/bin && \
	curl -sL -o ~/bin/gimme \
	https://raw.githubusercontent.com/travis-ci/gimme/master/gimme

RUN chmod +x ~/bin/gimme

RUN eval "$(~/bin/gimme ${GIMME_GO_VERSION})"

RUN mkdir /root/workspace

RUN git clone https://github.com/harmony-one/bls.git ${HMY_PATH}/bls

RUN git clone https://github.com/harmony-one/mcl.git ${HMY_PATH}/mcl

RUN cd ${HMY_PATH}/bls && make -j8 BLS_SWAP_G=1