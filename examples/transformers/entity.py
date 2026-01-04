from transformers import pipeline

ner = pipeline("ner", grouped_entities=True)

def run(input):
    entities = ner(input)
    return [entity['word'] for entity in entities]
