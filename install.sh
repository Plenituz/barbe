#!/bin/sh

# to run this:
# on mac:
# curl -fsSL https://hub.barbe.app/install.sh -o install.sh
# sh install.sh
# it will prompt you for you password without writing anything

set -e

binexe="barbe"
repo="Plenituz/barbe"
base="https://github.com/${repo}/releases/download"
version="${BARBE_VERSION:-latest}"
install_to="${BARBE_INSTALL_TO:-/usr/local}"
cd $install_to

cat /dev/null <<EOF
------------------------------------------------------------------------
https://github.com/client9/shlib - portable posix shell functions
Public domain - http://unlicense.org
https://github.com/client9/shlib/blob/master/LICENSE.md
but credit (and pull requests) appreciated.
------------------------------------------------------------------------
EOF
is_command() {
  command -v "$1" >/dev/null
}
echoerr() {
  echo "$@" 1>&2
}
log_prefix() {
  echo "$0"
}
_logp=7
log_set_priority() {
  _logp="$1"
}
log_priority() {
  if test -z "$1"; then
    echo "$_logp"
    return
  fi
  [ "$1" -le "$_logp" ]
}
log_tag() {
  case $1 in
    0) echo "emerg" ;;
    1) echo "alert" ;;
    2) echo "crit" ;;
    3) echo "err" ;;
    4) echo "warning" ;;
    5) echo "notice" ;;
    6) echo "info" ;;
    7) echo "debug" ;;
    *) echo "$1" ;;
  esac
}
log_debug() {
  log_priority 7 || return 0
  echoerr "$(log_prefix)" "$(log_tag 7)" "$@"
}
log_info() {
  log_priority 6 || return 0
  echoerr "$(log_prefix)" "$(log_tag 6)" "$@"
}
log_err() {
  log_priority 3 || return 0
  echoerr "$(log_prefix)" "$(log_tag 3)" "$@"
}
log_crit() {
  log_priority 2 || return 0
  echoerr "$(log_prefix)" "$(log_tag 2)" "$@"
}
uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    cygwin_nt*) os="windows" ;;
    mingw*) os="windows" ;;
    msys_nt*) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}
uname_os_check() {
  os=$(uname_os)
  case "$os" in
    darwin) return 0 ;;
    dragonfly) return 0 ;;
    freebsd) return 0 ;;
    linux) return 0 ;;
    android) return 0 ;;
    nacl) return 0 ;;
    netbsd) return 0 ;;
    openbsd) return 0 ;;
    plan9) return 0 ;;
    solaris) return 0 ;;
    windows) return 0 ;;
  esac
  log_crit "uname_os_check '$(uname -s)' got converted to '$os' which is not a GOOS value. Please file bug at https://github.com/client9/shlib"
  return 1
}
uname_arch_check() {
  arch=$(uname_arch)
  case "$arch" in
    386) return 0 ;;
    amd64) return 0 ;;
    arm64) return 0 ;;
    armv5) return 0 ;;
    armv6) return 0 ;;
    armv7) return 0 ;;
    ppc64) return 0 ;;
    ppc64le) return 0 ;;
    mips) return 0 ;;
    mipsle) return 0 ;;
    mips64) return 0 ;;
    mips64le) return 0 ;;
    s390x) return 0 ;;
    amd64p32) return 0 ;;
  esac
  log_crit "uname_arch_check '$(uname -m)' got converted to '$arch' which is not a GOARCH value.  Please file bug report at https://github.com/client9/shlib"
  return 1
}
untar() {
  tarball=$1
  case "${tarball}" in
    *.tar.gz | *.tgz) tar --no-same-owner -xzf "${tarball}" ;;
    *.tar) tar --no-same-owner -xf "${tarball}" ;;
    *.zip) unzip "${tarball}" ;;
    *)
      log_err "untar unknown archive format for ${tarball}"
      return 1
      ;;
  esac
}
http_download_curl() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    code=$(curl -w '%{http_code}' -sL -o "$local_file" "$source_url")
  else
    code=$(curl -w '%{http_code}' -sL -H "$header" -o "$local_file" "$source_url")
  fi
  if [ "$code" != "200" ]; then
    log_debug "http_download_curl received HTTP status $code"
    return 1
  fi
  return 0
}
http_download_wget() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    wget -q -O "$local_file" "$source_url"
  else
    wget -q --header "$header" -O "$local_file" "$source_url"
  fi
}
http_download() {
  log_debug "http_download $2"
  if is_command curl; then
    http_download_curl "$@"
    return
  elif is_command wget; then
    http_download_wget "$@"
    return
  fi
  log_crit "http_download unable to find wget or curl"
  return 1
}
http_copy() {
  tmp=$(mktemp)
  http_download "${tmp}" "$1" "$2" || return 1
  body=$(cat "$tmp")
  rm -f "${tmp}"
  echo "$body"
}
github_release() {
  owner_repo=$1
  version=$2
  test -z "$version" && version="latest"
  giturl="https://github.com/${owner_repo}/releases/${version}"
  json=$(http_copy "$giturl" "Accept:application/json")
  test -z "$json" && return 1
  version=$(echo "$json" | tr -s '\n' ' ' | sed 's/.*"tag_name":"//' | sed 's/".*//')
  test -z "$version" && return 1
  echo "$version"
}
hash_sha256() {
  TARGET=${1:-/dev/stdin}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$TARGET" 2>/dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl -dst openssl dgst -sha256 "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f a
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}
hash_sha256_verify() {
  TARGET=$1
  checksums=$2
  if [ -z "$checksums" ]; then
    log_err "hash_sha256_verify checksum file not specified in arg2"
    return 1
  fi
  BASENAME=${TARGET##*/}
  want=$(grep "${BASENAME}" "${checksums}" 2>/dev/null | tr '\t' ' ' | cut -d ' ' -f 1)
  if [ -z "$want" ]; then
    log_err "hash_sha256_verify unable to find checksum for '${TARGET}' in '${checksums}'"
    return 1
  fi
  got=$(hash_sha256 "$TARGET")
  if [ "$want" != "$got" ]; then
    log_err "hash_sha256_verify checksum for '$TARGET' did not verify ${want} vs $got"
    return 1
  fi
}
cat /dev/null <<EOF
------------------------------------------------------------------------
End of functions from https://github.com/client9/shlib
------------------------------------------------------------------------
EOF

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    cygwin_nt*) os="windows" ;;
    mingw*) os="windows" ;;
    msys_nt*) os="windows" ;;
  esac
  echo "$os"
}

uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo "$arch"
}

base_url() {
    os="$(uname_os)"
    arch="$(uname_arch)"
    version=$(github_release "${repo}" "${version}")
    url="${base}/${version}"
    echo "$url"
}

archive() {
    os="$(uname_os)"
    arch="$(uname_arch)"
    name="${os}_${arch}.zip"
    echo "$name"
}

execute() {
    base_url="$(base_url)"
    archive="$(archive)"
    archive_url="${base_url}/${archive}"
    bin_dir="${BIN_DIR:-./bin}"

    tmpdir=$(mktemp -d)
    log_debug "downloading files into ${tmpdir}"
    http_download "${tmpdir}/${archive}" "${archive_url}"
    srcdir="${tmpdir}"
    (cd "${tmpdir}" && untar "${archive}")
    test ! -d "${bin_dir}" && install -d "${bin_dir}"
    install "${srcdir}/${binexe}" "${bin_dir}"
    # remove quarantine attribute on macOS, until we can sign the binary
    test "$(uname_os)" = "darwin" && (xattr -d com.apple.quarantine "${bin_dir}/${binexe}" || true)
    log_info "installed ${bin_dir}/${binexe}"
    rm -rf "${tmpdir}"

    # for mac only, install brew if required, then install docker
    if [ "$(uname_os)" = "darwin" ]; then
        if ! is_command brew
        then
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        fi
        if ! is_command docker
        then
            brew install docker
        fi
        # try to run docker info, if it doesn't work, with the message "Cannot connect to the Docker daemon", then install colima using brew
        if ! docker info
        then
          # ask the user if they want us to install colima for them. If the script has a "-y" argument, then we assume yes
          if [[ "$@" != *"-y"* ]]
          then
            read -p "No container runtime is running. Would you like to install/start Colima (if you're not sure, yes is a good answer)? You can also install or start the Docker runtime yourself and re-run this script at https://docs.docker.com/desktop/install/mac-install/. (y/n) " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]
            then
              if ! is_command colima
              then
                  brew install colima
              fi
              colima start
            fi
          else
            if ! is_command colima
            then
                brew install colima
            fi
            colima start
          fi
        fi
    fi

    # same as above but for linux
    if [ "$(uname_os)" = "linux" ]; then
        if ! is_command docker
        then
            # ask the user if they want us to install docker for them
            if [[ "$@" != *"-y"* ]]
            then
              read -p "No container runtime is running. Would you like to install/start Docker (if you're not sure, yes is a good answer)? (y/n) " -n 1 -r
              echo
              if [[ $REPLY =~ ^[Yy]$ ]]
              then
                  if ! is_command docker
                  then
                      curl -fsSL https://get.docker.com -o get-docker.sh
                      sh get-docker.sh
                  fi
                  # start docker, make it work on most linux distros
                  sudo systemctl start docker || sudo service docker start || sudo service docker.io start
              fi
            else
              if ! is_command docker
              then
                  curl -fsSL https://get.docker.com -o get-docker.sh
                  sh get-docker.sh
              fi
              # start docker, make it work on most linux distros
              sudo systemctl start docker || sudo service docker start || sudo service docker.io start
            fi
        fi
    fi
}

execute "$1"