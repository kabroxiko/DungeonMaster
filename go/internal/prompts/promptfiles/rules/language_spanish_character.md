---
name: language-spanish-character
description: Spanish language directive for character-sheet generation only, with no campaign blocks.
---

OBLIGATORIO: RESPONDE SÓLO EN ESPAÑOL para TODO el contenido legible por humanos en la **hoja de personaje** de esta respuesta —incluyendo brief_backstory, background, texto de raza/clase/subclase, líneas de armor, equipment, weapons[].name, languages— tanto si la salida es YAML estructurado como texto libre. Los nombres de las claves pueden seguir convención técnica; los números en `coinage` (pp, gp, ep, sp, cp) no se traducen. **Esta petición solo debe producir `playerCharacter`** — no incluyas `campaignSpec` ni otros bloques de campaña. Mantén el YAML válido.

**Nombres propios en la hoja (`playerCharacter.name`, etc.):** escríbelos **en inglés** con ortografía inglesa y registro de fantasía épica (inventados o evocadores, no ciudades o famosos del mundo real). La **prosa** del trasfondo va en español natural alrededor de esos nombres.

**`brief_backstory`:** debe ser la **historia de su vida** en prosa **diegética**: solo ficción del mundo (orígenes, familia, formación, oficio o costumbres). **No** menciones **nivel**, **nivel 1**, **hoja de personaje**, **jugador**, **D&D**, **manual**, **clase** o **trasfondo** como términos de juego, ni planes de futuro en segunda persona. Los campos técnicos van en el YAML; el relato no debe sonar a resumen de reglas. Debe integrar orígenes y formación antes de cualquier episodio puntual.

El campo **`background`** (título del trasfondo de D&D 5e que ve el jugador en la hoja) debe ir **en español de mesa**, como en el Manual del Jugador en español — el título impreso en español para el trasfondo elegido, **nunca** el título en inglés del PHB cuando el idioma de sesión es español.

Los nombres de equipo del manual en inglés NO son excepción: en armor, equipment y weapons no deben quedar cadenas en inglés tipo PHB; usa **español de mesa** para el equipo. brief_backstory y equipo deben ir en el mismo idioma.

**Equipo (`equipment`):** **nunca** repitas en **`equipment`** el nombre de un arma que ya tenga fila en **`weapons`**. **`equipment`** es lo que queda después de separar armas, armadura llevada, herramientas PHB y cantidades de munición. Debe incluir **siempre** una línea para **dónde lleva las monedas** (monedero, bolsa de cinturón, etc.). Si **`ammunition`** no está vacío, otra línea para el **recipiente de munición al disparar**; la mochila sola no sustituye esa línea.

En `languages`, usa **solo** lenguajes del Manual del Jugador D&D 5e con **nombres de mesa en español** según el MJ. No añadas idiomas del mundo real como entradas de hoja ni grafías incorrectas.
