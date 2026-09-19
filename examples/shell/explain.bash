# explain shell integration for bash.
#
# Prefix any command with `??` to see what it does before running it:
#
#   ?? rm -rf ./build
#
# explain prints its verdict and explanation, then asks whether to run the
# command. Nothing runs unless you say so.
#
# Install by sourcing this file from ~/.bashrc:
#
#   source /path/to/explain/examples/shell/explain.bash

explain-confirm() {
  local cmd="$*"
  [[ -n "$cmd" ]] || return 0

  explain -- "$cmd"

  local reply
  read -r -p "
Run it? [y/N] " reply
  [[ "$reply" == [yY]* ]] || { echo "not run"; return 130; }

  history -s -- "$cmd"
  eval -- "$cmd"
}

alias '??'='explain-confirm '
