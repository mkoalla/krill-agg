import typesafe_sdk as ts


def str_answer(a: ts.Answer) -> str:
    t = type(a)
    if t is ts.NoulAnswer:
        noul = a.noul  # type: ignore
        prefix = "No"
        if abs(0.5 - noul) <= 0.1:
            prefix = "Uncertain"
        elif noul > 0.5:
            prefix = "Yes"
        #
        return f"{prefix}: True @ {noul:.2f}"
    elif t is ts.ChoiceAnswer:
        ans: ts.ChoiceAnswer = a  # type: ignore
        probs = sorted(
            [(i, ans.probabilities[i]) for i in ans.probabilities],
            key=lambda x: x[1],
            reverse=True,
        )
        return f"With {ans.confidence:.2f} confidence:\n\t{'\n\t'.join(f'{p[0]}: {p[1]:.2f}' for p in probs)}"
    elif t is ts.ScoreAnswer:
        ans: ts.ScoreAnswer = a  # type: ignore
        probs = sorted(
            [(i, ans.probabilities[i], ans.legend[i]) for i in ans.legend],
            key=lambda x: x[1],
            reverse=True,
        )
        return f"With {ans.confidence:.2f} confidence:\n\t{'\n\t'.join(f'{p[2]}: {p[1]:.2f}' for p in probs)}"
    return f"Unknown {t}: {a}"


class TSClientWrapper(ts.AsyncTypeSafeClient):
    query_cap = 50
    topic_cap = 50

    async def check_validity(self, query: str, topic: str):
        return await self.system_one(
            state={"q": query, "t": topic},
            questions={
                "Intelligibility": ts.Noul(
                    instructions="Is `q` intelligible?",
                    criteria=ts.NoulCriteria(
                        true="able to be reasonably interpreted lexically, \
                            it does not have to make complete logical sense.",
                        false="unable to infer intent, ranging from \
                            grammatical nonsense to an unfinished statement \
                            lacking a clear direction",
                    ),
                ),
                # "Translation": ts.Noul(
                #     instructions="Does `q` require translation?",
                #     criteria=ts.NoulCriteria(
                #         true="consists of words used in English, or commonly as\
                #              part of `t`, while also including descriptors/\
                #             nouns borrowed directly from other languages.",
                #         false="if not a direct component of `t`, AND it likely \
                #             needs to be translated to determine further \
                #             information about query",
                #     ),
                # ),
                "Relevance": ts.Noul(
                    instructions="Is `q` relevant as a way to filter\
                         content about `t`?",
                    criteria=ts.NoulCriteria(
                        true="most likely talking about or referencing `t`, \
                            and is intended to act as a search-query or keyword\
                             filter for it.",
                        false="is probably not talking about `t`, or is not asking\
                             a question relevant to categorizing `t`.",
                    ),
                ),
            },
        )

    async def test_query(self):
        res = await self.check_validity(
            input("Query: ")[: self.query_cap],
            input("Topic: ")[: self.topic_cap],
        )
        return "\n\n".join(
            f"{r}\n{str_answer(res.answers[r])}" for r in res.answers
        )
