"""Exercise API contracts without downloading models or contacting Chroma."""

import os
import unittest
from types import SimpleNamespace
from unittest.mock import patch

os.environ["USER_API_KEY"] = "zzfuse-test-user"
os.environ["ADMIN_API_KEY"] = "zzfuse-test-admin"

from fastapi.testclient import TestClient

import deps
import main
import sentencer_api


class ServiceTests(unittest.TestCase):
    def setUp(self):
        sentencer_api.model_cache.clear()
        self.client_context = TestClient(main.app)
        self.client = self.client_context.__enter__()
        self.user_headers = {"X-API-Key": "zzfuse-test-user"}
        self.admin_headers = {"X-API-Key": "zzfuse-test-admin"}

    def tearDown(self):
        self.client_context.__exit__(None, None, None)
        sentencer_api.model_cache.clear()

    def test_missing_and_placeholder_keys_are_rejected(self):
        for headers in ({}, {"X-API-Key": "your-api-key-here"}):
            with self.subTest(headers=headers):
                response = self.client.get("/sentencer/health", headers=headers)
                self.assertEqual(response.status_code, 401)
        self.assertEqual(
            self.client.get("/sentencer/health", headers=self.user_headers).status_code,
            200,
        )

    def test_unconfigured_keys_deny_access(self):
        with patch.dict(deps.API_KEYS, {}, clear=True):
            response = self.client.get("/sentencer/health", headers=self.admin_headers)
        self.assertEqual(response.status_code, 401)

    def test_embedding_shape_validation_and_cache(self):
        encoded = SimpleNamespace(tolist=lambda: [[0.1, 0.2], [0.3, 0.4]])
        model = SimpleNamespace(encode=lambda *args, **kwargs: encoded)
        with patch("sentencer_api.SentenceTransformer", return_value=model) as load:
            for _ in range(2):
                response = self.client.post(
                    "/sentencer/embed",
                    headers=self.user_headers,
                    json={"texts": ["first", "second"], "model_name": "test-model"},
                )
                self.assertEqual(response.status_code, 200)
                self.assertEqual(response.json()["embeddings"], [[0.1, 0.2], [0.3, 0.4]])
                self.assertEqual(response.json()["model_name"], "test-model")
            load.assert_called_once()
        response = self.client.post(
            "/sentencer/embed", headers=self.user_headers, json={"texts": []}
        )
        self.assertEqual(response.status_code, 422)

    def test_admin_check_precedes_chroma_connection(self):
        with patch("chroma_api.chromadb.HttpClient") as connect:
            response = self.client.delete(
                "/chroma/delete-collection",
                headers=self.user_headers,
                params={"name": "test", "chroma_url": "http://localhost:8010"},
            )
            self.assertEqual(response.status_code, 403)
            connect.assert_not_called()

    def test_chroma_tls_cache_and_shutdown(self):
        fake = SimpleNamespace(heartbeat=lambda: 1, list_collections=lambda: [])
        with patch("chroma_api.chromadb.HttpClient", return_value=fake) as connect:
            for url in ("https://localhost:8010", "https://localhost:8010", "http://localhost:8010"):
                response = self.client.get(
                    "/chroma/list-collections",
                    headers=self.user_headers,
                    params={"chroma_url": url},
                )
                self.assertEqual(response.status_code, 200)
            self.assertEqual(connect.call_count, 2)
            self.assertTrue(connect.call_args_list[0].kwargs["ssl"])
            self.assertFalse(connect.call_args_list[1].kwargs["ssl"])
        self.assertEqual(len(main.app.state.chroma_clients), 2)
        self.client_context.__exit__(None, None, None)
        self.assertEqual(main.app.state.chroma_clients, {})
        self.client_context = TestClient(main.app)
        self.client_context.__enter__()

    def test_invalid_chroma_urls_return_client_error(self):
        for url in ("not-a-url", "http://localhost:invalid", "ftp://localhost:8010", "http://[bad"):
            with self.subTest(url=url):
                response = self.client.get(
                    "/chroma/list-collections",
                    headers=self.user_headers,
                    params={"chroma_url": url},
                )
                self.assertEqual(response.status_code, 400)


if __name__ == "__main__":
    unittest.main()
