INSERT INTO "admin"."config_vectorstore_provider"("id","name","provider","endpoint","user","database","embedding_id","batch_size","flush_interval_ms","max_retries","max_open_conns","max_idle_conns","conn_max_lifetime_ms","batch_error_policy","max_concurrent_batches","vendor_specific_options","embedding_batch_group_signals","description","metadata","created_at","updated_at")
VALUES
(1,E'pgvector_large',E'pgvector',E'',E'',E'',1,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{}',E'2025-07-04 17:06:21.762802+00',E'2025-07-04 20:07:10.345899+00'),
(2,E'pgvector_small',E'pgvector',E'',E'',E'',2,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{}',E'2025-07-04 17:06:21.762802+00',E'2025-07-04 20:07:10.345899+00'),
(3,E'pgvector_ollama',E'pgvector',E'',E'',E'',4,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{}',E'2025-07-04 17:06:21.762802+00',E'2025-07-04 20:07:10.345899+00'),
(4,E'pgvector_openai',E'pgvector',E'',E'',E'',5,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{}',E'2025-07-04 17:06:21.762802+00',E'2025-07-04 20:07:10.345899+00'),
(5,E'qdrant',E'qdrant',E'http://192.0.2.1:6333',E'',E'',4,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{}',E'2025-07-04 17:06:21.762802+00',E'2025-07-05 21:40:19.00787+00'),
(6,E'chroma',E'chroma',E'http://192.0.2.1:8010',E'',E'',4,100,200,3,25,5,3600000,E'continue',10,E'{}',TRUE,E'Default vector store configuration',E'{"chroma_integration_endpoint": "http://localhost:8000"}',E'2025-07-04 17:06:21.762802+00',E'2025-07-06 20:37:31.753895+00');



