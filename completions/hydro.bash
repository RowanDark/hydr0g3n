# bash completion for hydro
_hydro_completions()
{
    local cur opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts="--beginner --burp-export --burp-host --concurrency --filter-size --follow-redirects --help --match-status --method --no-baseline --output --output-format --pre-hook --profile --resume --run-id --show-similarity --similarity-threshold --timeout -h -u -w"

    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
        return 0
    fi
}

complete -F _hydro_completions hydro
