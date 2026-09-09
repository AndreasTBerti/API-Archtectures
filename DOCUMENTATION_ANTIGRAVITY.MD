# Documentação Antigravity

**Prompt Original:**
> Analise o porque que o soma está dando problema

**Resumo da Ação:**
A função `soma` inserida continha problemas de sintaxe que impediam a compilação do projeto em Go:
1. Um *handler* do Gin obrigatoriamente precisa ter a assinatura `func(c *gin.Context)`. Não se pode adicionar os parâmetros (`a int, b int`) diretamente na assinatura, nem definir um tipo de retorno (como `int`).
2. Para recuperar variáveis que vêm na URL (como em `/soma/:a/:b`), é necessário usar `c.Param("nome")`, que retorna uma string.
3. Essas strings precisam ser convertidas para número inteiro usando o pacote `strconv` (`strconv.Atoi`).
4. Ao passar a função pro roteador (`router.GET`), devemos passar a referência da função (`soma`) e não chamá-la na mesma linha (`soma()`).

**Código Adicionado/Modificado (`main.go`):**
```go
import (
	"strconv" // pacote importado
    ...
)

// Função refatorada
func soma(c *gin.Context) {
	a, errA := strconv.Atoi(c.Param("a"))
	b, errB := strconv.Atoi(c.Param("b"))

	if errA != nil || errB != nil {
		c.JSON(400, gin.H{"error": "Parâmetros 'a' e 'b' devem ser numéricos"})
		return
	}

	c.JSON(200, gin.H{
		"soma": a + b,
	})
}

...
// Corrigido no main()
router.GET("/soma/:a/:b", soma)
```

---

**Prompt Original:**
> type UserRepository interface { ... }
> Me explique essa parte por favor 

**Resumo da Ação:**
O usuário solicitou uma explicação sobre o trecho de código em Go que define a interface `UserRepository`.

**Resposta/Explicação:**
Este código define uma **Interface** chamada `UserRepository` na linguagem Go. 
Interfaces funcionam como "contratos". Isso significa que qualquer estrutura (struct) no código que queira ser considerada um "Repositório de Usuário" (por exemplo, um repositório que salva no PostgreSQL, no MySQL, ou um *Mock* para testes) precisará obrigatoriamente implementar essas 5 funções (métodos).

Esse é o famoso **Padrão Repository (Repository Pattern)**, muito usado para separar a lógica de negócio do acesso a dados (banco de dados).

Detalhando cada método do contrato:
1. `Findall() ([]model.User, error)`: Retorna uma lista (slice) de todos os usuários (`[]model.User`) e um possível erro.
2. `FindById(id uint) (*model.User, error)`: Recebe um ID (inteiro positivo) e retorna um ponteiro para um usuário (`*model.User`) – o ponteiro é útil pois permite retornar `nil` caso não encontre – acompanhado de um erro.
3. `Create(user model.User) (model.User, error)`: Recebe um objeto de usuário para salvar, e retorna o usuário criado (normalmente já preenchido com o ID gerado pelo banco) e um erro.
4. `Update(user model.User) (model.User, error)`: Recebe um usuário modificado, atualiza no banco e retorna o usuário atualizado e um erro.
5. `Delete(user model.User) (model.User, error)`: Recebe um usuário a ser deletado e retorna o próprio usuário e um erro.

---

**Prompt Original:**
> Implemente o resto da classe usuário. Adicione os endpoints no swagger também.
> Armazene tudo em um sqlite local.
> dentro da pasta projeto rest
> Faça separado (Handler)
> rest.db (Nome do banco)

**Resumo da Ação:**
Finalizamos a implementação do recurso de Usuário ("classe" usuário) incluindo Repositório, Handler, Rotas, Banco de Dados e Swagger.

1. **Repositório (`internals/repository/user.repo.go`)**: Implementamos os métodos `Create`, `Update` e `Delete` utilizando o ORM Gorm.
2. **Banco de Dados**: Instalamos a biblioteca `gorm.io/driver/sqlite` via comando `go get` para dar suporte ao SQLite no Gorm.
3. **Handler (`internals/handler/user.handler.go`)**: Criamos este novo arquivo com as lógicas HTTP para operações GET, POST, PUT, e DELETE sobre usuários. Inserimos as devidas anotações do Swagger em cada uma das funções para possibilitar a documentação dos parâmetros, retornos esperados, etc.
4. **Configuração e Rotas (`main.go`)**: Modificamos o entrypoint principal para:
   - Inicializar a conexão com o arquivo local SQLite chamado `rest.db`.
   - Realizar o `AutoMigrate` para criar as tabelas.
   - Instanciar e injetar as dependências (Repo e Handler).
   - Registrar as novas rotas do Swagger (ex: `/users`, `/users/:id`).
5. **Swagger**: Rodamos o comando `swag init` na pasta do projeto para atualizar os arquivos gerados automaticamente (`swagger.json`, `docs.go`).

Todos os códigos foram aplicados e o banco local `rest.db` será criado quando a aplicação inicializar.

---

**Prompt Original / Erro:**
> Falha ao conectar no banco de dados sqlite:Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub

**Resumo da Ação:**
O driver original do Gorm para SQLite (`gorm.io/driver/sqlite`) utiliza internamente a biblioteca `go-sqlite3`, que requer CGO habilitado (compilação de código C) para funcionar nativamente, o que causou o problema ao executar `go run` no Windows sem um compilador C compatível previamente instalado e com CGO desabilitado.

**Resolução:**
Para resolver isso sem a necessidade de configurar CGO no Windows, instalamos um driver de SQLite "Pure Go" (escrito totalmente em Go), que é suportado pelo Gorm: `github.com/glebarez/sqlite`.

1. Instalamos a nova biblioteca: `go get github.com/glebarez/sqlite`
2. Alteramos a importação no arquivo `main.go`, trocando `gorm.io/driver/sqlite` por `github.com/glebarez/sqlite`.
3. Executamos um `go mod tidy` para garantir que os módulos estão corretos.

---

**Prompt Original / Erro:**
> Failed to fetch. Possible Reasons: CORS. Network Failure. URL scheme must be "http" or "https" for CORS request. (No Swagger UI)

**Resumo da Ação:**
Esse erro no Swagger UI acontece quando o cliente tenta executar um endpoint usando a interface "Try it out" e a requisição é bloqueada pelo navegador devido às políticas de CORS (Cross-Origin Resource Sharing), ou simplesmente porque a aplicação não está respondendo (Network Failure). 

**Resolução:**
Adicionamos o middleware oficial de CORS do Gin para que o servidor aceite as requisições que partem da interface de documentação sem bloqueios de segurança do navegador.
1. Instalamos o pacote de CORS do Gin via terminal: `go get github.com/gin-contrib/cors`
2. Modificamos o arquivo `main.go` importando o `github.com/gin-contrib/cors`.
3. Injetamos o middleware padrão logo após a criação do router com `router.Use(cors.Default())`.
4. Executamos `go mod tidy` para limpar e atualizar os módulos Go.

---

**Prompt Original / Erro:**
> `user_pass := user.Hash_pass`
> `user.Hash_pass, err := bcrypt.GenerateFromPassword([]byte(user_pass), bcrypt.DefaultCost)`
> `err := r.db.Create(&user).Error`
> Erro: non-name user.Hash_pass on left side of :=

**Resumo da Ação:**
O usuário tentou implementar o hash de senha utilizando a biblioteca `bcrypt`, mas cometeu alguns erros de sintaxe típicos da linguagem Go:
1. Usou `:=` tendo uma propriedade de struct `user.Hash_pass` do lado esquerdo, o que não é permitido em Go.
2. Usou `:=` na declaração de `err := r.db.Create(...)` logo após ter instanciado a mesma variável de erro na linha anterior, além do conflito de tipagem, pois `GenerateFromPassword` devolve `[]byte` mas o campo da struct esperava uma `string`.

**Resolução:**
Corrigimos a sintaxe no arquivo `user.repo.go`:
- Salvamos o resultado da função `GenerateFromPassword` numa nova variável (`hashedPass`).
- Fizemos o cast do slice de bytes `string(hashedPass)` de volta pra o campo da struct.
- Reaproveitamos o operador `=` no recebimento do erro do banco ao invés de usar `:=` novamente. 
- Rodamos `go mod tidy` para baixar formalmente o pacote da `golang.org/x/crypto/bcrypt`.

---

**Prompt Original / Erro:**
> Não quero que minh api retorne a senha hasheada 

**Resumo da Ação:**
O usuário pediu para que o campo de senha criptografada (`Hash_pass`) parasse de ser exibido nas respostas das requisições (como listar usuários, buscar por ID ou criar um novo usuário). Houve uma tentativa de mascarar isso no handler retornando `user.Nome, user.ID`, mas isso também gerou um erro de sintaxe.

**Resolução:**
A forma correta e mais robusta no ecossistema Go de evitar que um campo da estrutura vaze para a resposta HTTP é usando as *struct tags* do pacote nativo de JSON (`encoding/json`):
1. Fomos no arquivo `internals/model/user.model.go` e adicionamos a tag `` json:"-" `` no campo `Hash_pass`. Isso diz explicitamente ao Go: "nunca transforme esse campo em JSON".
2. Voltamos no `user.handler.go` na rota de criação e limpamos a tentativa de criar uma variável secundária (`information`). Agora ele retorna diretamente a estrutura `createdUser` (pois o campo da senha será ignorado automaticamente na hora de formatar a resposta).
3. Atualizamos a documentação visual rodando novamente o comando `swag init`, já que a representação do modelo de Usuário foi alterada.

---

**Prompt Original:**
> `Hash_pass string `gorm:"type:varchar(255);not null" json:"-"``
> Essas informações entre `` são para o swagger?

**Resumo da Ação:**
O usuário questionou sobre o funcionamento das informações entre crases (`` ` ``) nas propriedades das structs na linguagem Go e se elas serviam para o Swagger.

**Resposta/Explicação:**
Essas informações entre crases na linguagem Go são chamadas de **Struct Tags** (Tags de Estrutura). Elas não são exclusivas do Swagger, mas servem como "metadados" (informações extras) lidas por diversas bibliotecas durante a execução do programa ou compilação.

No caso do exemplo `gorm:"type:varchar(255);not null" json:"-"`:
- A parte `gorm:"..."` é lida apenas pelo **GORM** (o ORM de banco de dados). Ele usa isso para saber como criar a coluna lá no SQLite (dizendo que é um varchar de 255 caracteres e não pode ser nulo).
- A parte `json:"..."` é lida pela biblioteca nativa do Go que converte os dados para JSON (`encoding/json`) para quando os dados vão pro seu Frontend/Cliente. O traço `-` significa "ignore este campo no JSON".

O **Swagger** (`swag init`) entra como uma "terceira pessoa" que apenas lê o seu código fonte. Por ser muito esperto, quando ele vê a tag `json:"-"`, ele compreende que a sua API real nunca vai devolver esse campo e, portanto, ele também o oculta na documentação visual para manter o site igual à realidade!

---

**Prompt Original / Erro:**
> `users := h.repo.Findall()`
> `hasUser := slices.Contains(users, user.Nome)`
> Qual o jeito certo de implementar isso? (Check de usuário duplicado)

**Resumo da Ação:**
O usuário tentou validar se já existia um usuário cadastrado com o mesmo nome antes de criá-lo. A lógica de baixar todos os usuários (`Findall`) e usar `slices.Contains` em memória, além de gerar erros de compilação (devido a comparações de tipos incompatíveis), é uma má prática de performance em banco de dados, visto que todos os registros são trazidos para a RAM desnecessariamente.

**Resolução:**
O jeito certo de validar se um registro existe no Padrão Repository é delegar a busca para o banco de dados (que é extremamente otimizado pra isso) através de um método específico.

1. Adicionamos na interface `UserRepository` (no `user.repo.go`) um novo contrato: `FindByNome(nome string) (*model.User, error)`.
2. Implementamos o método `FindByNome` usando a função `.Where("nome = ?", nome).First(&user)` do Gorm, que vai direto ao banco buscar apenas o registro correspondente, retornando um erro de *Not Found* caso não exista.
3. Removemos a importação inútil de `"slices"` no cabeçalho do `user.handler.go` que geraria erro no compilador.
4. Alteramos a rota de *Create* do handler para utilizar `existingUser, _ := h.repo.FindByNome(user.Nome)`. Se `existingUser != nil`, o handler bloqueia a inserção e retorna imediatamente um erro HTTP 409 (Conflict), que é o status HTTP ideal para colisão de dados já existentes.

---

**Prompt Original:**
> Sempre que eu faço alguma alteração, preciso dar control-c e rodar novamente, mas isso demora. Existe algo que faça ele ir atualizando sozinho?

**Resumo da Ação:**
O usuário solicitou uma maneira de habilitar o *Hot/Live Reloading* na sua aplicação Go para que ela seja recompilada e reiniciada automaticamente a cada mudança feita no código fonte (evitando fechar e abrir o terminal).

**Resolução:**
O padrão da comunidade em Go para esse comportamento é uma ferramenta chamada **Air**.
1. Instalamos a versão mais recente do Air globalmente no ambiente usando: `go install github.com/air-verse/air@latest`.
2. Executamos o inicializador da ferramenta (`go run github.com/air-verse/air@latest init`) na raiz do projeto para que ele gerasse o arquivo `.air.toml`, que contém as regras de recompilação.
3. Agora basta utilizar o Air para rodar a aplicação em vez de usar `go run main.go`.

---

**Prompt Original / Erro:**
> From https://github.com/AndreasTBerti/API-Archtectures
>  * branch            main       -> FETCH_HEAD
> error: Your local changes to the following files would be overwritten by merge:
>         projeto-soap/main.py
> Please commit your changes or stash them before you merge.
> Aborting
> Merge with strategy ort failed.

**Resumo da Ação / Diagnóstico:**
O Git impediu a execução do comando `git pull` porque o arquivo `projeto-soap/main.py` possui alterações locais não salvas/comitadas (verificado via `git diff`: uma anotação `#Projeto em REST se trata de um gerenciador de produtos`), enquanto os commits mais recentes vindos do repositório remoto (`origin/main`) também alteram esse diretório. O Git recusa a mesclagem automaticamente para não apagar o conteúdo local sem autorização prévia.

**Resolução:**
Existem três opções para resolver esse impasse:
- **Opção 1 (Recomendada - Descartar anotação não intencional):** Se a alteração em `projeto-soap/main.py` foi apenas um teste/anotação temporária, basta descartá-la:
  ```bash
  git restore projeto-soap/main.py
  git pull origin main
  git push origin main
  ```
- **Opção 2 (Guardar alterações temporariamente com Stash):** Guarda as alterações locais na memória do Git, baixa as atualizações remotas e reaplica a alteração:
  ```bash
  git stash
  git pull origin main
  git stash pop
  ```
- **Opção 3 (Comitar a alteração local antes do pull):** Salva a alteração em um commit antes de fazer o merge:
  ```bash
  git add projeto-soap/main.py
  git commit -m "chore: atualizacao em projeto-soap"
  git pull origin main
  git push origin main
  ```
