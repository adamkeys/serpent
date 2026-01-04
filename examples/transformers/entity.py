from transformers import pipeline

ner = pipeline("ner", grouped_entities=True)
entities = ner(input)

values = []
for entity in entities:
    values.append(entity['word'])

result = values
