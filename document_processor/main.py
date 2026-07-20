from __future__ import annotations

import argparse
import logging

from app.grpc.server import configure_parser, serve


def main() -> None:
    parser = argparse.ArgumentParser(prog="myai-document-processor")
    subparsers = parser.add_subparsers(dest="command", required=True)
    worker_parser = subparsers.add_parser("worker", help="run a managed gRPC document worker")
    configure_parser(worker_parser)
    args = parser.parse_args()

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    if args.command == "worker":
        serve(args)


if __name__ == "__main__":
    main()
