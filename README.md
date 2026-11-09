# miv

Um editor modal pequeno em Go, inspirado nos comandos essenciais do Vim.
Implementação independente: não executa nem incorpora o Vim em C que está
em `../../vim/vim`. O código original serviu como referência de organização
e comportamento; este projeto não pretende oferecer compatibilidade completa.

## Rodar

Requer Go 1.23 ou superior e um terminal com suporte a UTF-8.

```sh
make build
./bin/miv caminho/do/arquivo
# ou abrir vários arquivos:
./bin/miv main.go README.md
# ou executar sem gerar o binário em bin/:
go run ./cmd/miv caminho/do/arquivo
```

Sem argumento, começa com um buffer vazio. Use `:w nome` para salvar.
O binário funciona sem Go instalado. Neste ambiente, o compilador foi
baixado para `/tmp/go`: use `PATH=/tmp/go/bin:$PATH make build` enquanto
ele não estiver no seu PATH.

## Comandos

`Esc` retorna ao modo normal; `Ctrl-c` também cancela a operação atual.
As operações aceitam contagens, por exemplo `5j`, `3dw`, `d2w` e `2dd`.

| Ação | Teclas |
| --- | --- |
| Mover | `h j k l` ou setas |
| Palavras | `w b e` |
| Início, primeiro caractere, fim da linha | `0 ^ $` |
| Primeira/última linha; linha N | `gg`, `G`, `Ngg`, `NG`, `:N` |
| Rolar 15 linhas | `Ctrl-d`, `Ctrl-u`, PageDown, PageUp |
| Buscar caractere na linha | `f{caractere}`, `F{caractere}`, `t{caractere}`, `T{caractere}` |
| Parêntese/colchete/chave correspondente | `%` |
| Inserir | `i a I A o O` |
| Apagar, alterar, copiar | `d{movimento}`, `c{movimento}`, `y{movimento}` |
| Linha inteira | `dd cc yy` |
| Até o fim da linha | `D C Y` |
| Palavra e objetos de texto | `diw`, `daw`, `ciw`, `ci"`, `ci'`, `ci(`, `di[`, `da{` etc. |
| Apagar caractere, substituir e alterar | `x`, `r{caractere}`, `s` |
| Seleção por caractere/linha | `v`, `V`; depois `d`, `c`, `y`, `>`, `<` |
| Colar depois/antes | `p`, `P` |
| Desfazer/refazer | `u`, `Ctrl-r` |
| Repetir última edição | `.` |
| Juntar linhas; inverter maiúsculas/minúsculas | `J`, `~` |
| Indentar/desindentar | `>>`, `<<` |
| Busca literal para frente/trás | `/texto`, `?texto`, Enter |
| Próxima/anterior ocorrência | `n`, `N` |
| Buscar palavra sob o cursor | `*` |
| Encontrar arquivo | `Ctrl-p`, `Espaço f`, `:files` |
| Alternar buffers | `gt`, `gT`, `:bn`, `:bp`, `:b N` |
| Ir ao buffer N/último | `Espaço 1` a `Espaço 9`, `Espaço 0` |
| Listar buffers | `:ls` |
| Abrir/criar arquivo em outro buffer | `:e caminho` |
| Salvar | `:w`, `:w caminho`, `Espaço s` |
| Salvar e fechar buffer | `:wq` |
| Fechar buffer; sair de todos | `:q`, `:qa` |
| Descartar alterações ao fechar | `:q!`, `:qa!` |

O seletor de arquivos faz busca por subsequência: `smg` encontra
`src/main.go`. Use setas ou `Ctrl-j`/`Ctrl-k`, Enter para abrir e Esc
para cancelar. Procura a partir do diretório em que o editor foi iniciado,
inclui arquivos ocultos, ignora `.git`, `node_modules`, `target`, `vendor`
e `.cache`, e limita a listagem a 20 mil arquivos. Não segue symlinks
durante a busca.

No modo de inserção: Backspace, Delete, Enter com indentação automática,
Tab com espaços, setas e `Ctrl-w` para apagar a palavra anterior.
A colagem do terminal usa bracketed paste para preservar o conteúdo
sem interpretar comandos ou acrescentar indentação.

## Preferências do seu workflow

- `Y` copia até o fim da linha, conforme seu `.vimrc`.
- YAML, shell e JSON usam indentação de 2 espaços; os outros arquivos, 4.
- Clipboard do sistema com `wl-copy`/`wl-paste` no Wayland,
  `xclip` ou `xsel` no X11, ou `pbcopy`/`pbpaste` no macOS.
  Sem uma dessas ferramentas, copiar/colar continua funcionando dentro do editor.
- Arquivos abertos ficam em buffers, com abas no topo, sem descartar alterações ao trocar.
- Linhas longas usam rolagem horizontal; não há quebra visual automática.

O editor tem preferências fixas e não lê `.vimrc` nem a configuração do Neovim.
Os atalhos próprios foram simplificados: salvar é `Espaço s`; o seu
`Espaço f s` não está implementado.

## Arquivos e limites

Aceita texto UTF-8 de até 16 MiB na abertura. Preserva LF/CRLF uniforme e
a presença ou ausência de quebra de linha no fim do arquivo. Cada edição
guarda um snapshot em memória, com limite de 200 entradas de undo por buffer;
o foco é editar arquivos pequenos e médios.

O salvamento usa um arquivo temporário no mesmo diretório e rename.
Preserva as permissões de um arquivo existente e o destino de symlinks
válidos. Detecta alterações externas e colisões com arquivos existentes;
`:w!` permite sobrescrever explicitamente. Como usa rename, não preserva
hardlinks, proprietário de outro usuário, ACLs ou atributos estendidos.
Não há swap, autosave nem recuperação após encerramento inesperado.

Ficaram fora desta versão: LSP, plugins/Vimscript/Lua, debugger,
syntax highlighting, splits, seleção em bloco, macros, múltiplos
registradores, regex/substituição e grafemas complexos como unidade de edição.
Movimentos e posições usam pontos de código Unicode. A semântica de comandos
é um subconjunto simplificado do Vim, incluindo o tratamento da última linha.

## Desenvolvimento

```sh
make test
make check
make smoke  # Python 3 e pseudoterminal Unix
```

O núcleo em `internal/editor` não depende do terminal. `cmd/miv` conecta
os eventos, desenha a tela com tcell e acessa o clipboard. Os testes cobrem
edição, histórico, arquivos, busca, seletor, renderização e resize; o smoke
test abre o binário num PTY, edita UTF-8, cola várias linhas, salva, fecha
e verifica a restauração do terminal.
