# Partir el turno de combate en dos llamadas

## Hoy: una llamada con herramientas

El modelo decide la estructura entera del turno —abre el encuentro, elige a quién se tira, juega la
ronda y la cierra— y el servidor tapa los huecos con heurísticas sobre el texto.

```mermaid
flowchart TD
    A["Mensaje del jugador"] --> B["LLAMADA 1 · modelo con 4 herramientas + JSON"]
    B --> B1["decide si hay pelea y abre el encuentro"]
    B --> B2["elige a quién se tira y juega la ronda"]
    B --> B3["cierra el turno y escribe la prosa"]
    B1 --> C["Servidor: heurísticas sobre la respuesta"]
    B2 --> C
    B3 --> C
    C --> C1["needsMore: falta que actúe un enemigo"]
    C --> C2["roundSpent: jugó dos rondas"]
    C --> C3["termina en pregunta"]
    C --> C4["contradice el hp del registro"]
    C1 --> D["Respuesta al jugador"]
    C2 --> D
    C3 --> D
    C4 --> D
```

## Propuesta: el motor decide la estructura, el modelo escribe

```mermaid
sequenceDiagram
    autonumber
    actor J as Jugador
    participant S as Servidor
    participant I as Modelo intérprete
    participant M as Motor
    participant N as Modelo narrador

    J->>S: acción declarada en prosa
    S->>S: leer registro — participantes, hp, ronda, orden
    S->>I: mensaje + hoja + registro por nombres
    I-->>S: kind, target, arma, dado
    Note over I,S: sin herramientas · JSON estricto · barata
    opt la pelea empieza en este turno
        I-->>S: participantes con su hp y su CA
        S->>M: abrir encuentro y tirar iniciativa
    end
    S->>M: resolver la ronda entera en orden de iniciativa
    M-->>S: líneas con los números — daño, hp, quién sigue en pie
    S->>M: avanzar ronda y fijar awaiting al personaje del jugador
    S->>N: instrucciones + cola del historial + BLOQUE DE RESULTADOS
    N-->>S: narration + encounterState
    Note over N,S: sin herramientas · sin JSON de estado que copiar
    S->>S: comprobaciones como diagnóstico, sin reintento silencioso
    S-->>J: ronda cerrada y la pregunta de la siguiente
```

## Con varios jugadores: la respuesta se corta donde un humano tiene que hablar

El puntero del motor recorre el orden de iniciativa. Si el actor es el personaje de un jugador, la
respuesta se detiene y se le pide la declaración **a ese jugador**. Si el actor es un PNJ, la IA decide
la acción de ese PNJ **en su propio turno**, con el registro delante, y el motor la resuelve antes de
pasar al siguiente. Una misma respuesta puede contener varios turnos de PNJ seguidos; lo que nunca
contiene es el turno de un jugador, porque ese lo abre su mensaje.

```mermaid
sequenceDiagram
    autonumber
    actor V as Varek · jugador 1
    actor L as Lyra · jugador 2
    participant S as Servidor
    participant IA as Modelo IA
    participant M as Motor

    Note over S: ronda 2 · orden Varek 18 · Goblin 16 · Lyra 14 · Hobgoblin 12
    V->>S: declara su acción
    S->>M: resuelve el turno de Varek
    M-->>S: números del turno
    S->>S: siguiente actor Goblin — no es humano
    S->>IA: decide el turno del Goblin, con el registro delante
    IA-->>S: acción del Goblin, a quién ataca
    S->>M: resuelve el turno del Goblin
    M-->>S: números del turno
    S->>S: siguiente actor Lyra — humano, se corta aquí
    S->>IA: narra los turnos jugados, con el bloque de resultados
    IA-->>S: narración de Varek y del Goblin
    S-->>L: pide la declaración de Lyra, que abre su turno
```

El bucle del puntero, en una vista:

```mermaid
flowchart TD
    P["puntero al siguiente actor del orden"] --> Q{"quién es el actor"}
    Q -->|"personaje de un jugador"| H["se corta la respuesta y se le pide la declaración"]
    Q -->|"PNJ"| N["la IA decide la acción de ese PNJ en su turno"]
    N --> R["el motor la resuelve y aplica hp"]
    R --> S2{"queda alguien en pie"}
    S2 -->|"no"| E["fin del combate y narración del cierre"]
    S2 -->|"sí"| P2["avanzar el puntero"]
    P2 --> P
    H --> W["el jugador responde y se retoma el puntero"]
    W --> P
```

## El reparto, en una vista

```mermaid
flowchart LR
    subgraph SE["Servidor"]
        S1["lee el registro"]
        S2["interpreta la acción del jugador"]
        S3["resuelve cada turno"]
        S4["corta donde hay que preguntar"]
        S5["diagnostica contradicciones"]
    end
    subgraph MO["Modelo"]
        M1["convierte la prosa en una acción"]
        M2["decide el turno de cada PNJ"]
        M3["escribe la escena de lo que ya pasó"]
    end
    S1 --> M1
    M1 --> S2
    S2 --> S3
    S3 --> M2
    M2 --> S3
    S3 --> M3
    M3 --> S4
    S4 --> S5
```

## Qué desaparece

- El bucle de herramientas en combate: `open_encounter`, `roll`, `request_action`, `encounter`.
- Las heurísticas que tapaban al modelo: `roundSpent`, el cierre por texto, la pregunta añadida,
  el reintento por ronda incompleta. El motor ya sabe cuándo acaba una ronda y a quién le toca.
- El riesgo de que el modelo juegue dos rondas o se salte un turno: no tiene con qué.

## Qué queda

- `combatTools` deja de ser una API para el modelo y pasa a ser la API interna del motor: misma
  lógica de dados, hp, hojas e iniciativa, sin reescribir nada de eso.
- El registro sigue siendo la única verdad, y lleva quién espera (`awaiting`) y qué ronda corre.
- `encounterState` en la respuesta del narrador queda como contraste; su ausencia se diagnostica con
  su causa, sin reintento automático.

## Coste

- Una llamada más por turno de combate, más una por cada turno de PNJ que quepa en la respuesta. A
  cambio, la narración es una llamada simple: sin herramientas, con `json_object` disponible otra vez
  y con un bloque de resultados delante en lugar de un registro que copiar.
- El intérprete tiene que acertar la acción declarada; es una tarea mucho más pequeña que la actual
  (una frase a un objeto) y ya está probada en el camino que se usa cuando el host rechaza herramientas.

## Decisiones abiertas

- **Turno de PNJ: una llamada por turno o una por bloque.** Por turno es lo más fiel —cada criatura
  decide viendo lo que acaba de pasar, y puede reaccionar a la caída de un aliado— y cuesta una llamada
  por PNJ. Por bloque decide de golpe todos los PNJ que actúan antes del siguiente jugador: menos
  latencia, menos reacción en cadena.
- **Quién controla los otros personajes.** Con varios jugadores humanos, cada uno declara el suyo. Si
  un personaje del grupo no tiene jugador en la mesa, hay que decidir si lo lleva la IA en su turno
  como un PNJ, o si su turno espera siempre a un humano.
- **Quién propone las estadísticas de los enemigos nuevos** —hp y CA— cuando empieza la pelea: la
  llamada del intérprete, o una tabla del sistema con valores por tipo de criatura que el intérprete
  solo nombre.
