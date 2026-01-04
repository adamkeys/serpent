from transformers import pipeline

ner = pipeline("ner", grouped_entities=True)
text = "Apple was founded by Steve Jobs in Cupertino, California."
entities = ner(text)

values = []
for entity in entities:
    values.append(entity['word'])

result = values
