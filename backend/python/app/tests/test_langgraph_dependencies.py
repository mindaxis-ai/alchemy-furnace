def test_langgraph_dependencies_import():
    from langgraph.graph import StateGraph
    from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver
    from langchain_core.language_models.chat_models import BaseChatModel
    from langchain_openai import ChatOpenAI
    from langchain_deepseek import ChatDeepSeek
    from langchain_ollama import ChatOllama

    assert StateGraph and AsyncSqliteSaver and BaseChatModel
    assert ChatOpenAI and ChatDeepSeek and ChatOllama
