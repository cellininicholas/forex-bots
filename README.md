# Some bots

## OANDA short term bot

### Params

C, M, R, S, P, L

### ALgo

- Iterate over pairs without current trade
- Get candles for frequency C
- Read the momentum over period M
- Read the RSI over period R
- Read the max/min value over period S
- Enter into trade with TP and SL, proportional to max-min by factor P and L
