# explain shell integration for zsh.
#
# Prefix any command with `??` to see what it does before running it:
#
#   ?? rm -rf ./build
#
# explain prints its verdict and explanation, then asks whether to run the
# command. Nothing runs unless you say so.
#
# Install by sourcing this file from ~/.zshrc:
#
#   source /path/to/explain/examples/shell/explain.zsh

explain-confirm() {
  local cmd="$*"
  [[ -n "$cmd" ]] || return 0

  explain -- "$cmd"

  # The verdict is already on screen; ask before running, defaulting to no.
  local reply
  read -r "reply?
Run it? [y/N] "
  [[ "$reply" == [yY]* ]] || { print "not run"; return 130; }

  # Record it in history as if it had been typed, then run it.
  print -s -- "$cmd"
  eval -- "$cmd"
}

# `??` is not a valid function name, so alias it to one. The trailing space
# lets the rest of the line expand as usual.
alias -- '??'='explain-confirm '
