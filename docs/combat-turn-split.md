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

## El reparto, en una vista

```mermaid
flowchart LR
    subgraph SE["Servidor"]
        S1["lee el registro"]
        S2["interpreta la acción"]
        S3["resuelve la ronda"]
        S4["cierra la ronda y espera al jugador"]
        S5["diagnostica contradicciones"]
    end
    subgraph MO["Modelo"]
        M1["convierte la prosa en una acción"]
        M2["escribe la escena de lo que ya pasó"]
    end
    S1 --> M1
    M1 --> S2
    S2 --> S3
    S3 --> M2
    M2 --> S4
    S4 --> S5
```

## Qué desaparece

- El bucle de herramientas en combate: `open_encounter`, `roll`, `request_action`, `encounter`.
- Las heurísticas que tapaban al modelo: `roundSpent`, el cierre por texto, la pregunta añadida,
  el reintento por ronda incompleta. El motor ya sabe cuándo acaba una ronda: no hay nada que deducir.
- El riesgo de que el modelo juegue dos rondas: no tiene con qué.

## Qué queda

- `combatTools` deja de ser una API para el modelo y pasa a ser la API interna del motor: misma
  lógica de dados, hp, hojas e iniciativa, sin reescribir nada de eso.
- El registro sigue siendo la única verdad. `encounterState` en la respuesta del narrador queda como
  contraste, y su ausencia se diagnostica con su causa, como ahora.

## Coste

- Una llamada más por turno de combate, la del intérprete, a cambio de que la narración sea una
  llamada simple: sin herramientas, con `json_object` disponible otra vez y con un bloque de
  resultados delante en lugar de un registro que copiar.
- El intérprete tiene que acertar la acción declarada; es una tarea mucho más pequeña que la actual
  (una frase a un objeto) y ya está probada en el camino que se usa cuando el host rechaza herramientas.

## Decisión abierta

Quién propone las estadísticas de los enemigos nuevos —hp y CA— cuando la pelea empieza: la llamada
del intérprete, o una tabla del sistema con valores por tipo de criatura que el intérprete solo nombre.
