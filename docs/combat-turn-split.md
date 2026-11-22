# Partir el turno de combate en dos llamadas

## Hoy: una llamada con herramientas

```
mensaje del jugador
   │
   └─► LLAMADA 1 (con 4 herramientas + JSON)
         el modelo decide TODO: abre el encuentro, elige a quién tira,
         juega la ronda, cierra el turno y escribe la prosa
   │
   └─► SERVIDOR: heurísticas para tapar los huecos
         needsMore · roundSpent · conflictos · ¿termina en pregunta? · hp drift
```

Todo lo que ha fallado viene de aquí: el modelo decide la estructura (y no la decide bien).

## Propuesta: el motor decide, el modelo escribe

```
                        SERVIDOR (dueño de las reglas y del registro)
 ┌───────────────────────────────────────────────────────────────────────────────┐
 │ mensaje del jugador                                                           │
 │   │                                                                           │
 │   ├─(1)─ ¿hay pelea abierta?  registro: participantes · hp · ronda · orden     │
 │   │                                                                           │
 │   ├─(2)─ LLAMADA 1 · intérprete   [ya existe: runCombatEngine + declare.md]    │
 │   │        in : mensaje + hoja + registro (nombres, hp, ronda)                 │
 │   │        out: { kind: attack|heal|save|check, target, arma, dado }           │
 │   │             y, si la pelea empieza aquí, los participantes con su hp/ac    │
 │   │        sin herramientas · JSON estricto · barata                           │
 │   │                                                                           │
 │   ├─(3)─ MOTOR resuelve la ronda ENTERA en orden de iniciativa                 │
 │   │        → líneas con los números que quedan (daño, hp, quién sigue en pie)  │
 │   │        → ronda+1 · awaiting = personaje del jugador                        │
 │   │                                                                           │
 │   ├─(4)─ LLAMADA 2 · narrador   [ya existe: GenerateResponse + insertSystemBlock]
 │   │        in : instrucciones + cola del historial + BLOQUE DE RESULTADOS      │
 │   │        out: { narration, encounterState }                                  │
 │   │        sin herramientas · sin JSON de estado que copiar · json_object OK    │
 │   │                                                                           │
 │   └─(5)─ comprobaciones (diagnóstico, no reintento silencioso)                 │
 │            ¿contradice el registro? ¿omitió el estado? → log con la causa      │
 └───────────────────────────────────────────────────────────────────────────────┘
```

## Qué desaparece

- El bucle de herramientas en combate: `open_encounter`, `roll`, `request_action`, `encounter`.
- Las heurísticas que tapaban al modelo: `roundSpent`, el cierre por texto, la pregunta añadida,
  el reintento por ronda incompleta. El motor ya sabe cuándo acaba una ronda: no hay nada que deducir.
- El riesgo de que el modelo juegue dos rondas: no tiene con qué.

## Qué queda

- `combatTools` deja de ser una API para el modelo y pasa a ser la API interna del motor
  (misma lógica de dados, hp, hojas, iniciativa: no se reescribe nada de eso).
- El registro sigue siendo la única verdad; `encounterState` en la respuesta del narrador queda
  como contraste (y su ausencia se diagnostica, como ahora).

## Coste

- Una llamada más por turno de combate (la del intérprete), a cambio de que la narración sea una
  llamada simple: sin herramientas, con `json_object` disponible otra vez, y con un bloque de
  resultados delante en lugar de un registro que copiar.
- El intérprete tiene que acertar la acción declarada; es una tarea mucho más pequeña que la actual
  (una frase → un objeto) y ya está probada en el camino sin herramientas.
