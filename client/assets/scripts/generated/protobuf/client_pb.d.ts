import * as $protobuf from "protobufjs";
import Long = require("long");

/** Namespace minimir. */
export namespace minimir {

    /** Namespace v1. */
    namespace v1 {

        /**
         * Properties of a RoleKey.
         * @deprecated Use minimir.v1.RoleKey.$Properties instead.
         */
        interface IRoleKey extends minimir.v1.RoleKey.$Properties {
        }

        /** Represents a RoleKey. */
        class RoleKey {

            /**
             * Constructs a new RoleKey.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RoleKey.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RoleKey role_id. */
            role_id: Uint8Array;

            /** RoleKey origin_server_id. */
            origin_server_id: Long;

            /**
             * Creates a new RoleKey instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RoleKey instance
             */
            static create(properties: minimir.v1.RoleKey.$Shape): minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape;
            static create(properties?: minimir.v1.RoleKey.$Properties): minimir.v1.RoleKey;

            /**
             * Encodes the specified RoleKey message. Does not implicitly {@link minimir.v1.RoleKey.verify|verify} messages.
             * @param message RoleKey message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RoleKey.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RoleKey message, length delimited. Does not implicitly {@link minimir.v1.RoleKey.verify|verify} messages.
             * @param message RoleKey message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RoleKey.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RoleKey message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape} RoleKey
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape;

            /**
             * Decodes a RoleKey message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape} RoleKey
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape;

            /**
             * Verifies a RoleKey message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RoleKey
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RoleKey {

            /** Properties of a RoleKey. */
            interface $Properties {

                /** RoleKey role_id */
                role_id?: (Uint8Array|null);

                /** RoleKey origin_server_id */
                origin_server_id?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RoleKey. */
            type $Shape = minimir.v1.RoleKey.$Properties;
        }

        /**
         * Properties of a GridPoint.
         * @deprecated Use minimir.v1.GridPoint.$Properties instead.
         */
        interface IGridPoint extends minimir.v1.GridPoint.$Properties {
        }

        /** Represents a GridPoint. */
        class GridPoint {

            /**
             * Constructs a new GridPoint.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.GridPoint.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** GridPoint x. */
            x: number;

            /** GridPoint y. */
            y: number;

            /**
             * Creates a new GridPoint instance using the specified properties.
             * @param [properties] Properties to set
             * @returns GridPoint instance
             */
            static create(properties: minimir.v1.GridPoint.$Shape): minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape;
            static create(properties?: minimir.v1.GridPoint.$Properties): minimir.v1.GridPoint;

            /**
             * Encodes the specified GridPoint message. Does not implicitly {@link minimir.v1.GridPoint.verify|verify} messages.
             * @param message GridPoint message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.GridPoint.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified GridPoint message, length delimited. Does not implicitly {@link minimir.v1.GridPoint.verify|verify} messages.
             * @param message GridPoint message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.GridPoint.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a GridPoint message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape} GridPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape;

            /**
             * Decodes a GridPoint message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape} GridPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape;

            /**
             * Verifies a GridPoint message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for GridPoint
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace GridPoint {

            /** Properties of a GridPoint. */
            interface $Properties {

                /** GridPoint x */
                x?: (number|null);

                /** GridPoint y */
                y?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a GridPoint. */
            type $Shape = minimir.v1.GridPoint.$Properties;
        }

        /**
         * Properties of an OperationContext.
         * @deprecated Use minimir.v1.OperationContext.$Properties instead.
         */
        interface IOperationContext extends minimir.v1.OperationContext.$Properties {
        }

        /** Represents an OperationContext. */
        class OperationContext {

            /**
             * Constructs a new OperationContext.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.OperationContext.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** OperationContext op_id. */
            op_id: Uint8Array;

            /** OperationContext command_seq. */
            command_seq: Long;

            /** OperationContext expected_revision. */
            expected_revision: Long;

            /** OperationContext expected_rule_version. */
            expected_rule_version: Long;

            /**
             * Creates a new OperationContext instance using the specified properties.
             * @param [properties] Properties to set
             * @returns OperationContext instance
             */
            static create(properties: minimir.v1.OperationContext.$Shape): minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape;
            static create(properties?: minimir.v1.OperationContext.$Properties): minimir.v1.OperationContext;

            /**
             * Encodes the specified OperationContext message. Does not implicitly {@link minimir.v1.OperationContext.verify|verify} messages.
             * @param message OperationContext message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.OperationContext.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified OperationContext message, length delimited. Does not implicitly {@link minimir.v1.OperationContext.verify|verify} messages.
             * @param message OperationContext message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.OperationContext.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an OperationContext message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape} OperationContext
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape;

            /**
             * Decodes an OperationContext message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape} OperationContext
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape;

            /**
             * Verifies an OperationContext message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for OperationContext
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace OperationContext {

            /** Properties of an OperationContext. */
            interface $Properties {

                /** OperationContext op_id */
                op_id?: (Uint8Array|null);

                /** OperationContext command_seq */
                command_seq?: (Long|null);

                /** OperationContext expected_revision */
                expected_revision?: (Long|null);

                /** OperationContext expected_rule_version */
                expected_rule_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an OperationContext. */
            type $Shape = minimir.v1.OperationContext.$Properties;
        }

        /**
         * Properties of an OperationResult.
         * @deprecated Use minimir.v1.OperationResult.$Properties instead.
         */
        interface IOperationResult extends minimir.v1.OperationResult.$Properties {
        }

        /** Represents an OperationResult. */
        class OperationResult {

            /**
             * Constructs a new OperationResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.OperationResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** OperationResult op_id. */
            op_id: Uint8Array;

            /** OperationResult command_seq. */
            command_seq: Long;

            /** OperationResult new_revision. */
            new_revision: Long;

            /** OperationResult error_code. */
            error_code: number;

            /** OperationResult pending. */
            pending: boolean;

            /**
             * Creates a new OperationResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns OperationResult instance
             */
            static create(properties: minimir.v1.OperationResult.$Shape): minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape;
            static create(properties?: minimir.v1.OperationResult.$Properties): minimir.v1.OperationResult;

            /**
             * Encodes the specified OperationResult message. Does not implicitly {@link minimir.v1.OperationResult.verify|verify} messages.
             * @param message OperationResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.OperationResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified OperationResult message, length delimited. Does not implicitly {@link minimir.v1.OperationResult.verify|verify} messages.
             * @param message OperationResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.OperationResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an OperationResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape} OperationResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape;

            /**
             * Decodes an OperationResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape} OperationResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape;

            /**
             * Verifies an OperationResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for OperationResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace OperationResult {

            /** Properties of an OperationResult. */
            interface $Properties {

                /** OperationResult op_id */
                op_id?: (Uint8Array|null);

                /** OperationResult command_seq */
                command_seq?: (Long|null);

                /** OperationResult new_revision */
                new_revision?: (Long|null);

                /** OperationResult error_code */
                error_code?: (number|null);

                /** OperationResult pending */
                pending?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an OperationResult. */
            type $Shape = minimir.v1.OperationResult.$Properties;
        }

        /**
         * Properties of a Heartbeat.
         * @deprecated Use minimir.v1.Heartbeat.$Properties instead.
         */
        interface IHeartbeat extends minimir.v1.Heartbeat.$Properties {
        }

        /** Represents a Heartbeat. */
        class Heartbeat {

            /**
             * Constructs a new Heartbeat.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.Heartbeat.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** Heartbeat client_monotonic_ms. */
            client_monotonic_ms: Long;

            /**
             * Creates a new Heartbeat instance using the specified properties.
             * @param [properties] Properties to set
             * @returns Heartbeat instance
             */
            static create(properties: minimir.v1.Heartbeat.$Shape): minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape;
            static create(properties?: minimir.v1.Heartbeat.$Properties): minimir.v1.Heartbeat;

            /**
             * Encodes the specified Heartbeat message. Does not implicitly {@link minimir.v1.Heartbeat.verify|verify} messages.
             * @param message Heartbeat message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.Heartbeat.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified Heartbeat message, length delimited. Does not implicitly {@link minimir.v1.Heartbeat.verify|verify} messages.
             * @param message Heartbeat message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.Heartbeat.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a Heartbeat message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape} Heartbeat
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape;

            /**
             * Decodes a Heartbeat message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape} Heartbeat
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape;

            /**
             * Verifies a Heartbeat message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for Heartbeat
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace Heartbeat {

            /** Properties of a Heartbeat. */
            interface $Properties {

                /** Heartbeat client_monotonic_ms */
                client_monotonic_ms?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a Heartbeat. */
            type $Shape = minimir.v1.Heartbeat.$Properties;
        }

        /**
         * Properties of a HeartbeatResult.
         * @deprecated Use minimir.v1.HeartbeatResult.$Properties instead.
         */
        interface IHeartbeatResult extends minimir.v1.HeartbeatResult.$Properties {
        }

        /** Represents a HeartbeatResult. */
        class HeartbeatResult {

            /**
             * Constructs a new HeartbeatResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.HeartbeatResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** HeartbeatResult server_unix_ms. */
            server_unix_ms: Long;

            /**
             * Creates a new HeartbeatResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns HeartbeatResult instance
             */
            static create(properties: minimir.v1.HeartbeatResult.$Shape): minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape;
            static create(properties?: minimir.v1.HeartbeatResult.$Properties): minimir.v1.HeartbeatResult;

            /**
             * Encodes the specified HeartbeatResult message. Does not implicitly {@link minimir.v1.HeartbeatResult.verify|verify} messages.
             * @param message HeartbeatResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.HeartbeatResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified HeartbeatResult message, length delimited. Does not implicitly {@link minimir.v1.HeartbeatResult.verify|verify} messages.
             * @param message HeartbeatResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.HeartbeatResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a HeartbeatResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape} HeartbeatResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape;

            /**
             * Decodes a HeartbeatResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape} HeartbeatResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape;

            /**
             * Verifies a HeartbeatResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for HeartbeatResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace HeartbeatResult {

            /** Properties of a HeartbeatResult. */
            interface $Properties {

                /** HeartbeatResult server_unix_ms */
                server_unix_ms?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a HeartbeatResult. */
            type $Shape = minimir.v1.HeartbeatResult.$Properties;
        }

        /**
         * Properties of a Batch.
         * @deprecated Use minimir.v1.Batch.$Properties instead.
         */
        interface IBatch extends minimir.v1.Batch.$Properties {
        }

        /** Represents a Batch. */
        class Batch {

            /**
             * Constructs a new Batch.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.Batch.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** Batch application_bodies. */
            application_bodies: Uint8Array[];

            /**
             * Creates a new Batch instance using the specified properties.
             * @param [properties] Properties to set
             * @returns Batch instance
             */
            static create(properties: minimir.v1.Batch.$Shape): minimir.v1.Batch & minimir.v1.Batch.$Shape;
            static create(properties?: minimir.v1.Batch.$Properties): minimir.v1.Batch;

            /**
             * Encodes the specified Batch message. Does not implicitly {@link minimir.v1.Batch.verify|verify} messages.
             * @param message Batch message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.Batch.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified Batch message, length delimited. Does not implicitly {@link minimir.v1.Batch.verify|verify} messages.
             * @param message Batch message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.Batch.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a Batch message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.Batch & minimir.v1.Batch.$Shape} Batch
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.Batch & minimir.v1.Batch.$Shape;

            /**
             * Decodes a Batch message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.Batch & minimir.v1.Batch.$Shape} Batch
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.Batch & minimir.v1.Batch.$Shape;

            /**
             * Verifies a Batch message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for Batch
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace Batch {

            /** Properties of a Batch. */
            interface $Properties {

                /** Batch application_bodies */
                application_bodies?: (Uint8Array[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a Batch. */
            type $Shape = minimir.v1.Batch.$Properties;
        }

        /**
         * Properties of a ConfigVersion.
         * @deprecated Use minimir.v1.ConfigVersion.$Properties instead.
         */
        interface IConfigVersion extends minimir.v1.ConfigVersion.$Properties {
        }

        /** Represents a ConfigVersion. */
        class ConfigVersion {

            /**
             * Constructs a new ConfigVersion.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ConfigVersion.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ConfigVersion rule_version. */
            rule_version: Long;

            /** ConfigVersion content_hash. */
            content_hash: string;

            /** ConfigVersion resource_version. */
            resource_version: Long;

            /** ConfigVersion protocol_version. */
            protocol_version: number;

            /**
             * Creates a new ConfigVersion instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ConfigVersion instance
             */
            static create(properties: minimir.v1.ConfigVersion.$Shape): minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape;
            static create(properties?: minimir.v1.ConfigVersion.$Properties): minimir.v1.ConfigVersion;

            /**
             * Encodes the specified ConfigVersion message. Does not implicitly {@link minimir.v1.ConfigVersion.verify|verify} messages.
             * @param message ConfigVersion message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ConfigVersion.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ConfigVersion message, length delimited. Does not implicitly {@link minimir.v1.ConfigVersion.verify|verify} messages.
             * @param message ConfigVersion message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ConfigVersion.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ConfigVersion message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape} ConfigVersion
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape;

            /**
             * Decodes a ConfigVersion message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape} ConfigVersion
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape;

            /**
             * Verifies a ConfigVersion message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ConfigVersion
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ConfigVersion {

            /** Properties of a ConfigVersion. */
            interface $Properties {

                /** ConfigVersion rule_version */
                rule_version?: (Long|null);

                /** ConfigVersion content_hash */
                content_hash?: (string|null);

                /** ConfigVersion resource_version */
                resource_version?: (Long|null);

                /** ConfigVersion protocol_version */
                protocol_version?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ConfigVersion. */
            type $Shape = minimir.v1.ConfigVersion.$Properties;
        }

        /**
         * Properties of a LocalLoginRequest.
         * @deprecated Use minimir.v1.LocalLoginRequest.$Properties instead.
         */
        interface ILocalLoginRequest extends minimir.v1.LocalLoginRequest.$Properties {
        }

        /** Represents a LocalLoginRequest. */
        class LocalLoginRequest {

            /**
             * Constructs a new LocalLoginRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.LocalLoginRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** LocalLoginRequest account. */
            account: string;

            /** LocalLoginRequest password. */
            password: string;

            /**
             * Creates a new LocalLoginRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns LocalLoginRequest instance
             */
            static create(properties: minimir.v1.LocalLoginRequest.$Shape): minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape;
            static create(properties?: minimir.v1.LocalLoginRequest.$Properties): minimir.v1.LocalLoginRequest;

            /**
             * Encodes the specified LocalLoginRequest message. Does not implicitly {@link minimir.v1.LocalLoginRequest.verify|verify} messages.
             * @param message LocalLoginRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.LocalLoginRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified LocalLoginRequest message, length delimited. Does not implicitly {@link minimir.v1.LocalLoginRequest.verify|verify} messages.
             * @param message LocalLoginRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.LocalLoginRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a LocalLoginRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape} LocalLoginRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape;

            /**
             * Decodes a LocalLoginRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape} LocalLoginRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape;

            /**
             * Verifies a LocalLoginRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for LocalLoginRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace LocalLoginRequest {

            /** Properties of a LocalLoginRequest. */
            interface $Properties {

                /** LocalLoginRequest account */
                account?: (string|null);

                /** LocalLoginRequest password */
                password?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a LocalLoginRequest. */
            type $Shape = minimir.v1.LocalLoginRequest.$Properties;
        }

        /**
         * Properties of a LoginResult.
         * @deprecated Use minimir.v1.LoginResult.$Properties instead.
         */
        interface ILoginResult extends minimir.v1.LoginResult.$Properties {
        }

        /** Represents a LoginResult. */
        class LoginResult {

            /**
             * Constructs a new LoginResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.LoginResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** LoginResult account_id. */
            account_id: Uint8Array;

            /** LoginResult account_token. */
            account_token: string;

            /** LoginResult expires_at_unix. */
            expires_at_unix: Long;

            /**
             * Creates a new LoginResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns LoginResult instance
             */
            static create(properties: minimir.v1.LoginResult.$Shape): minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape;
            static create(properties?: minimir.v1.LoginResult.$Properties): minimir.v1.LoginResult;

            /**
             * Encodes the specified LoginResult message. Does not implicitly {@link minimir.v1.LoginResult.verify|verify} messages.
             * @param message LoginResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.LoginResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified LoginResult message, length delimited. Does not implicitly {@link minimir.v1.LoginResult.verify|verify} messages.
             * @param message LoginResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.LoginResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a LoginResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape} LoginResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape;

            /**
             * Decodes a LoginResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape} LoginResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape;

            /**
             * Verifies a LoginResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for LoginResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace LoginResult {

            /** Properties of a LoginResult. */
            interface $Properties {

                /** LoginResult account_id */
                account_id?: (Uint8Array|null);

                /** LoginResult account_token */
                account_token?: (string|null);

                /** LoginResult expires_at_unix */
                expires_at_unix?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a LoginResult. */
            type $Shape = minimir.v1.LoginResult.$Properties;
        }

        /**
         * Properties of a RoleDirectoryRequest.
         * @deprecated Use minimir.v1.RoleDirectoryRequest.$Properties instead.
         */
        interface IRoleDirectoryRequest extends minimir.v1.RoleDirectoryRequest.$Properties {
        }

        /** Represents a RoleDirectoryRequest. */
        class RoleDirectoryRequest {

            /**
             * Constructs a new RoleDirectoryRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RoleDirectoryRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RoleDirectoryRequest origin_server_id. */
            origin_server_id: Long;

            /**
             * Creates a new RoleDirectoryRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RoleDirectoryRequest instance
             */
            static create(properties: minimir.v1.RoleDirectoryRequest.$Shape): minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape;
            static create(properties?: minimir.v1.RoleDirectoryRequest.$Properties): minimir.v1.RoleDirectoryRequest;

            /**
             * Encodes the specified RoleDirectoryRequest message. Does not implicitly {@link minimir.v1.RoleDirectoryRequest.verify|verify} messages.
             * @param message RoleDirectoryRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RoleDirectoryRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RoleDirectoryRequest message, length delimited. Does not implicitly {@link minimir.v1.RoleDirectoryRequest.verify|verify} messages.
             * @param message RoleDirectoryRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RoleDirectoryRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RoleDirectoryRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape} RoleDirectoryRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape;

            /**
             * Decodes a RoleDirectoryRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape} RoleDirectoryRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape;

            /**
             * Verifies a RoleDirectoryRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RoleDirectoryRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RoleDirectoryRequest {

            /** Properties of a RoleDirectoryRequest. */
            interface $Properties {

                /** RoleDirectoryRequest origin_server_id */
                origin_server_id?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RoleDirectoryRequest. */
            type $Shape = minimir.v1.RoleDirectoryRequest.$Properties;
        }

        /**
         * Properties of a RoleSummary.
         * @deprecated Use minimir.v1.RoleSummary.$Properties instead.
         */
        interface IRoleSummary extends minimir.v1.RoleSummary.$Properties {
        }

        /** Represents a RoleSummary. */
        class RoleSummary {

            /**
             * Constructs a new RoleSummary.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RoleSummary.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RoleSummary key. */
            key?: (minimir.v1.RoleKey.$Properties|null);

            /** RoleSummary name. */
            name: string;

            /** RoleSummary job. */
            job: number;

            /** RoleSummary gender. */
            gender: number;

            /** RoleSummary level. */
            level: number;

            /** RoleSummary slot. */
            slot: number;

            /** RoleSummary appearance_version. */
            appearance_version: Long;

            /**
             * Creates a new RoleSummary instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RoleSummary instance
             */
            static create(properties: minimir.v1.RoleSummary.$Shape): minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape;
            static create(properties?: minimir.v1.RoleSummary.$Properties): minimir.v1.RoleSummary;

            /**
             * Encodes the specified RoleSummary message. Does not implicitly {@link minimir.v1.RoleSummary.verify|verify} messages.
             * @param message RoleSummary message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RoleSummary.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RoleSummary message, length delimited. Does not implicitly {@link minimir.v1.RoleSummary.verify|verify} messages.
             * @param message RoleSummary message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RoleSummary.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RoleSummary message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape} RoleSummary
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape;

            /**
             * Decodes a RoleSummary message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape} RoleSummary
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape;

            /**
             * Verifies a RoleSummary message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RoleSummary
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RoleSummary {

            /** Properties of a RoleSummary. */
            interface $Properties {

                /** RoleSummary key */
                key?: (minimir.v1.RoleKey.$Properties|null);

                /** RoleSummary name */
                name?: (string|null);

                /** RoleSummary job */
                job?: (number|null);

                /** RoleSummary gender */
                gender?: (number|null);

                /** RoleSummary level */
                level?: (number|null);

                /** RoleSummary slot */
                slot?: (number|null);

                /** RoleSummary appearance_version */
                appearance_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RoleSummary. */
            type $Shape = minimir.v1.RoleSummary.$Properties;
        }

        /**
         * Properties of a RoleDirectoryResult.
         * @deprecated Use minimir.v1.RoleDirectoryResult.$Properties instead.
         */
        interface IRoleDirectoryResult extends minimir.v1.RoleDirectoryResult.$Properties {
        }

        /** Represents a RoleDirectoryResult. */
        class RoleDirectoryResult {

            /**
             * Constructs a new RoleDirectoryResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RoleDirectoryResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RoleDirectoryResult roles. */
            roles: minimir.v1.RoleSummary.$Properties[];

            /** RoleDirectoryResult route_version. */
            route_version: Long;

            /**
             * Creates a new RoleDirectoryResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RoleDirectoryResult instance
             */
            static create(properties: minimir.v1.RoleDirectoryResult.$Shape): minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape;
            static create(properties?: minimir.v1.RoleDirectoryResult.$Properties): minimir.v1.RoleDirectoryResult;

            /**
             * Encodes the specified RoleDirectoryResult message. Does not implicitly {@link minimir.v1.RoleDirectoryResult.verify|verify} messages.
             * @param message RoleDirectoryResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RoleDirectoryResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RoleDirectoryResult message, length delimited. Does not implicitly {@link minimir.v1.RoleDirectoryResult.verify|verify} messages.
             * @param message RoleDirectoryResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RoleDirectoryResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RoleDirectoryResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape} RoleDirectoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape;

            /**
             * Decodes a RoleDirectoryResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape} RoleDirectoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape;

            /**
             * Verifies a RoleDirectoryResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RoleDirectoryResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RoleDirectoryResult {

            /** Properties of a RoleDirectoryResult. */
            interface $Properties {

                /** RoleDirectoryResult roles */
                roles?: (minimir.v1.RoleSummary.$Properties[]|null);

                /** RoleDirectoryResult route_version */
                route_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RoleDirectoryResult. */
            type $Shape = minimir.v1.RoleDirectoryResult.$Properties;
        }

        /**
         * Properties of a CreateRoleRequest.
         * @deprecated Use minimir.v1.CreateRoleRequest.$Properties instead.
         */
        interface ICreateRoleRequest extends minimir.v1.CreateRoleRequest.$Properties {
        }

        /** Represents a CreateRoleRequest. */
        class CreateRoleRequest {

            /**
             * Constructs a new CreateRoleRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CreateRoleRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CreateRoleRequest origin_server_id. */
            origin_server_id: Long;

            /** CreateRoleRequest slot. */
            slot: number;

            /** CreateRoleRequest job. */
            job: number;

            /** CreateRoleRequest name. */
            name: string;

            /** CreateRoleRequest op_id. */
            op_id: Uint8Array;

            /**
             * Creates a new CreateRoleRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CreateRoleRequest instance
             */
            static create(properties: minimir.v1.CreateRoleRequest.$Shape): minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape;
            static create(properties?: minimir.v1.CreateRoleRequest.$Properties): minimir.v1.CreateRoleRequest;

            /**
             * Encodes the specified CreateRoleRequest message. Does not implicitly {@link minimir.v1.CreateRoleRequest.verify|verify} messages.
             * @param message CreateRoleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CreateRoleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CreateRoleRequest message, length delimited. Does not implicitly {@link minimir.v1.CreateRoleRequest.verify|verify} messages.
             * @param message CreateRoleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CreateRoleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CreateRoleRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape} CreateRoleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape;

            /**
             * Decodes a CreateRoleRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape} CreateRoleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape;

            /**
             * Verifies a CreateRoleRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CreateRoleRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CreateRoleRequest {

            /** Properties of a CreateRoleRequest. */
            interface $Properties {

                /** CreateRoleRequest origin_server_id */
                origin_server_id?: (Long|null);

                /** CreateRoleRequest slot */
                slot?: (number|null);

                /** CreateRoleRequest job */
                job?: (number|null);

                /** CreateRoleRequest name */
                name?: (string|null);

                /** CreateRoleRequest op_id */
                op_id?: (Uint8Array|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CreateRoleRequest. */
            type $Shape = minimir.v1.CreateRoleRequest.$Properties;
        }

        /**
         * Properties of an EntryTicketRequest.
         * @deprecated Use minimir.v1.EntryTicketRequest.$Properties instead.
         */
        interface IEntryTicketRequest extends minimir.v1.EntryTicketRequest.$Properties {
        }

        /** Represents an EntryTicketRequest. */
        class EntryTicketRequest {

            /**
             * Constructs a new EntryTicketRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EntryTicketRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EntryTicketRequest role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /**
             * Creates a new EntryTicketRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EntryTicketRequest instance
             */
            static create(properties: minimir.v1.EntryTicketRequest.$Shape): minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape;
            static create(properties?: minimir.v1.EntryTicketRequest.$Properties): minimir.v1.EntryTicketRequest;

            /**
             * Encodes the specified EntryTicketRequest message. Does not implicitly {@link minimir.v1.EntryTicketRequest.verify|verify} messages.
             * @param message EntryTicketRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EntryTicketRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EntryTicketRequest message, length delimited. Does not implicitly {@link minimir.v1.EntryTicketRequest.verify|verify} messages.
             * @param message EntryTicketRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EntryTicketRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EntryTicketRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape} EntryTicketRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape;

            /**
             * Decodes an EntryTicketRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape} EntryTicketRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape;

            /**
             * Verifies an EntryTicketRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EntryTicketRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EntryTicketRequest {

            /** Properties of an EntryTicketRequest. */
            interface $Properties {

                /** EntryTicketRequest role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntryTicketRequest. */
            type $Shape = minimir.v1.EntryTicketRequest.$Properties;
        }

        /**
         * Properties of an EntryTicketResult.
         * @deprecated Use minimir.v1.EntryTicketResult.$Properties instead.
         */
        interface IEntryTicketResult extends minimir.v1.EntryTicketResult.$Properties {
        }

        /** Represents an EntryTicketResult. */
        class EntryTicketResult {

            /**
             * Constructs a new EntryTicketResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EntryTicketResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EntryTicketResult ticket. */
            ticket: string;

            /** EntryTicketResult game_wss_url. */
            game_wss_url: string;

            /** EntryTicketResult run_server_id. */
            run_server_id: Long;

            /** EntryTicketResult route_version. */
            route_version: Long;

            /** EntryTicketResult expires_at_unix. */
            expires_at_unix: Long;

            /**
             * Creates a new EntryTicketResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EntryTicketResult instance
             */
            static create(properties: minimir.v1.EntryTicketResult.$Shape): minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape;
            static create(properties?: minimir.v1.EntryTicketResult.$Properties): minimir.v1.EntryTicketResult;

            /**
             * Encodes the specified EntryTicketResult message. Does not implicitly {@link minimir.v1.EntryTicketResult.verify|verify} messages.
             * @param message EntryTicketResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EntryTicketResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EntryTicketResult message, length delimited. Does not implicitly {@link minimir.v1.EntryTicketResult.verify|verify} messages.
             * @param message EntryTicketResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EntryTicketResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EntryTicketResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape} EntryTicketResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape;

            /**
             * Decodes an EntryTicketResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape} EntryTicketResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape;

            /**
             * Verifies an EntryTicketResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EntryTicketResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EntryTicketResult {

            /** Properties of an EntryTicketResult. */
            interface $Properties {

                /** EntryTicketResult ticket */
                ticket?: (string|null);

                /** EntryTicketResult game_wss_url */
                game_wss_url?: (string|null);

                /** EntryTicketResult run_server_id */
                run_server_id?: (Long|null);

                /** EntryTicketResult route_version */
                route_version?: (Long|null);

                /** EntryTicketResult expires_at_unix */
                expires_at_unix?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntryTicketResult. */
            type $Shape = minimir.v1.EntryTicketResult.$Properties;
        }

        /**
         * Properties of an EnterLineRequest.
         * @deprecated Use minimir.v1.EnterLineRequest.$Properties instead.
         */
        interface IEnterLineRequest extends minimir.v1.EnterLineRequest.$Properties {
        }

        /** Represents an EnterLineRequest. */
        class EnterLineRequest {

            /**
             * Constructs a new EnterLineRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EnterLineRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnterLineRequest account_ticket. */
            account_ticket: string;

            /**
             * Creates a new EnterLineRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnterLineRequest instance
             */
            static create(properties: minimir.v1.EnterLineRequest.$Shape): minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape;
            static create(properties?: minimir.v1.EnterLineRequest.$Properties): minimir.v1.EnterLineRequest;

            /**
             * Encodes the specified EnterLineRequest message. Does not implicitly {@link minimir.v1.EnterLineRequest.verify|verify} messages.
             * @param message EnterLineRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EnterLineRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnterLineRequest message, length delimited. Does not implicitly {@link minimir.v1.EnterLineRequest.verify|verify} messages.
             * @param message EnterLineRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EnterLineRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnterLineRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape} EnterLineRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape;

            /**
             * Decodes an EnterLineRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape} EnterLineRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape;

            /**
             * Verifies an EnterLineRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EnterLineRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnterLineRequest {

            /** Properties of an EnterLineRequest. */
            interface $Properties {

                /** EnterLineRequest account_ticket */
                account_ticket?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnterLineRequest. */
            type $Shape = minimir.v1.EnterLineRequest.$Properties;
        }

        /**
         * Properties of an EnterLineResult.
         * @deprecated Use minimir.v1.EnterLineResult.$Properties instead.
         */
        interface IEnterLineResult extends minimir.v1.EnterLineResult.$Properties {
        }

        /** Represents an EnterLineResult. */
        class EnterLineResult {

            /**
             * Constructs a new EnterLineResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EnterLineResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnterLineResult scene_epoch. */
            scene_epoch: Long;

            /** EnterLineResult baseline_id. */
            baseline_id: Long;

            /** EnterLineResult room_id. */
            room_id: Long;

            /** EnterLineResult line_id. */
            line_id: number;

            /** EnterLineResult resource_version. */
            resource_version: Long;

            /** EnterLineResult position. */
            position?: (minimir.v1.GridPoint.$Properties|null);

            /** EnterLineResult scene_kind. */
            scene_kind: number;

            /** EnterLineResult next_economic_command_seq. */
            next_economic_command_seq: Long;

            /** EnterLineResult player_handle. */
            player_handle: number;

            /** EnterLineResult server_transition_id. */
            server_transition_id: string;

            /**
             * Creates a new EnterLineResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnterLineResult instance
             */
            static create(properties: minimir.v1.EnterLineResult.$Shape): minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape;
            static create(properties?: minimir.v1.EnterLineResult.$Properties): minimir.v1.EnterLineResult;

            /**
             * Encodes the specified EnterLineResult message. Does not implicitly {@link minimir.v1.EnterLineResult.verify|verify} messages.
             * @param message EnterLineResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EnterLineResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnterLineResult message, length delimited. Does not implicitly {@link minimir.v1.EnterLineResult.verify|verify} messages.
             * @param message EnterLineResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EnterLineResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnterLineResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape} EnterLineResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape;

            /**
             * Decodes an EnterLineResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape} EnterLineResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape;

            /**
             * Verifies an EnterLineResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EnterLineResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnterLineResult {

            /** Properties of an EnterLineResult. */
            interface $Properties {

                /** EnterLineResult scene_epoch */
                scene_epoch?: (Long|null);

                /** EnterLineResult baseline_id */
                baseline_id?: (Long|null);

                /** EnterLineResult room_id */
                room_id?: (Long|null);

                /** EnterLineResult line_id */
                line_id?: (number|null);

                /** EnterLineResult resource_version */
                resource_version?: (Long|null);

                /** EnterLineResult position */
                position?: (minimir.v1.GridPoint.$Properties|null);

                /** EnterLineResult scene_kind */
                scene_kind?: (number|null);

                /** EnterLineResult next_economic_command_seq */
                next_economic_command_seq?: (Long|null);

                /** EnterLineResult player_handle */
                player_handle?: (number|null);

                /** EnterLineResult server_transition_id */
                server_transition_id?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnterLineResult. */
            type $Shape = minimir.v1.EnterLineResult.$Properties;
        }

        /**
         * Properties of an EnterWorldAck.
         * @deprecated Use minimir.v1.EnterWorldAck.$Properties instead.
         */
        interface IEnterWorldAck extends minimir.v1.EnterWorldAck.$Properties {
        }

        /** Represents an EnterWorldAck. */
        class EnterWorldAck {

            /**
             * Constructs a new EnterWorldAck.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EnterWorldAck.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnterWorldAck scene_epoch. */
            scene_epoch: Long;

            /** EnterWorldAck baseline_id. */
            baseline_id: Long;

            /**
             * Creates a new EnterWorldAck instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnterWorldAck instance
             */
            static create(properties: minimir.v1.EnterWorldAck.$Shape): minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape;
            static create(properties?: minimir.v1.EnterWorldAck.$Properties): minimir.v1.EnterWorldAck;

            /**
             * Encodes the specified EnterWorldAck message. Does not implicitly {@link minimir.v1.EnterWorldAck.verify|verify} messages.
             * @param message EnterWorldAck message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EnterWorldAck.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnterWorldAck message, length delimited. Does not implicitly {@link minimir.v1.EnterWorldAck.verify|verify} messages.
             * @param message EnterWorldAck message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EnterWorldAck.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnterWorldAck message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape} EnterWorldAck
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape;

            /**
             * Decodes an EnterWorldAck message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape} EnterWorldAck
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape;

            /**
             * Verifies an EnterWorldAck message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EnterWorldAck
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnterWorldAck {

            /** Properties of an EnterWorldAck. */
            interface $Properties {

                /** EnterWorldAck scene_epoch */
                scene_epoch?: (Long|null);

                /** EnterWorldAck baseline_id */
                baseline_id?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnterWorldAck. */
            type $Shape = minimir.v1.EnterWorldAck.$Properties;
        }

        /**
         * Properties of a MapTravelPrepare.
         * @deprecated Use minimir.v1.MapTravelPrepare.$Properties instead.
         */
        interface IMapTravelPrepare extends minimir.v1.MapTravelPrepare.$Properties {
        }

        /** Represents a MapTravelPrepare. */
        class MapTravelPrepare {

            /**
             * Constructs a new MapTravelPrepare.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.MapTravelPrepare.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MapTravelPrepare scene_epoch. */
            scene_epoch: Long;

            /** MapTravelPrepare travel_id. */
            travel_id: string;

            /** MapTravelPrepare destination. */
            destination: number;

            /**
             * Creates a new MapTravelPrepare instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MapTravelPrepare instance
             */
            static create(properties: minimir.v1.MapTravelPrepare.$Shape): minimir.v1.MapTravelPrepare & minimir.v1.MapTravelPrepare.$Shape;
            static create(properties?: minimir.v1.MapTravelPrepare.$Properties): minimir.v1.MapTravelPrepare;

            /**
             * Encodes the specified MapTravelPrepare message. Does not implicitly {@link minimir.v1.MapTravelPrepare.verify|verify} messages.
             * @param message MapTravelPrepare message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.MapTravelPrepare.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MapTravelPrepare message, length delimited. Does not implicitly {@link minimir.v1.MapTravelPrepare.verify|verify} messages.
             * @param message MapTravelPrepare message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.MapTravelPrepare.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MapTravelPrepare message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.MapTravelPrepare & minimir.v1.MapTravelPrepare.$Shape} MapTravelPrepare
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.MapTravelPrepare & minimir.v1.MapTravelPrepare.$Shape;

            /**
             * Decodes a MapTravelPrepare message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.MapTravelPrepare & minimir.v1.MapTravelPrepare.$Shape} MapTravelPrepare
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.MapTravelPrepare & minimir.v1.MapTravelPrepare.$Shape;

            /**
             * Verifies a MapTravelPrepare message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for MapTravelPrepare
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MapTravelPrepare {

            /** Properties of a MapTravelPrepare. */
            interface $Properties {

                /** MapTravelPrepare scene_epoch */
                scene_epoch?: (Long|null);

                /** MapTravelPrepare travel_id */
                travel_id?: (string|null);

                /** MapTravelPrepare destination */
                destination?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MapTravelPrepare. */
            type $Shape = minimir.v1.MapTravelPrepare.$Properties;
        }

        /**
         * Properties of a MapTravelPlan.
         * @deprecated Use minimir.v1.MapTravelPlan.$Properties instead.
         */
        interface IMapTravelPlan extends minimir.v1.MapTravelPlan.$Properties {
        }

        /** Represents a MapTravelPlan. */
        class MapTravelPlan {

            /**
             * Constructs a new MapTravelPlan.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.MapTravelPlan.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MapTravelPlan travel_id. */
            travel_id: string;

            /** MapTravelPlan from_scene_epoch. */
            from_scene_epoch: Long;

            /** MapTravelPlan target_map_id. */
            target_map_id: Long;

            /** MapTravelPlan target_scene_kind. */
            target_scene_kind: number;

            /** MapTravelPlan position. */
            position?: (minimir.v1.GridPoint.$Properties|null);

            /** MapTravelPlan expires_at_unix_ms. */
            expires_at_unix_ms: Long;

            /**
             * Creates a new MapTravelPlan instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MapTravelPlan instance
             */
            static create(properties: minimir.v1.MapTravelPlan.$Shape): minimir.v1.MapTravelPlan & minimir.v1.MapTravelPlan.$Shape;
            static create(properties?: minimir.v1.MapTravelPlan.$Properties): minimir.v1.MapTravelPlan;

            /**
             * Encodes the specified MapTravelPlan message. Does not implicitly {@link minimir.v1.MapTravelPlan.verify|verify} messages.
             * @param message MapTravelPlan message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.MapTravelPlan.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MapTravelPlan message, length delimited. Does not implicitly {@link minimir.v1.MapTravelPlan.verify|verify} messages.
             * @param message MapTravelPlan message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.MapTravelPlan.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MapTravelPlan message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.MapTravelPlan & minimir.v1.MapTravelPlan.$Shape} MapTravelPlan
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.MapTravelPlan & minimir.v1.MapTravelPlan.$Shape;

            /**
             * Decodes a MapTravelPlan message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.MapTravelPlan & minimir.v1.MapTravelPlan.$Shape} MapTravelPlan
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.MapTravelPlan & minimir.v1.MapTravelPlan.$Shape;

            /**
             * Verifies a MapTravelPlan message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for MapTravelPlan
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MapTravelPlan {

            /** Properties of a MapTravelPlan. */
            interface $Properties {

                /** MapTravelPlan travel_id */
                travel_id?: (string|null);

                /** MapTravelPlan from_scene_epoch */
                from_scene_epoch?: (Long|null);

                /** MapTravelPlan target_map_id */
                target_map_id?: (Long|null);

                /** MapTravelPlan target_scene_kind */
                target_scene_kind?: (number|null);

                /** MapTravelPlan position */
                position?: (minimir.v1.GridPoint.$Properties|null);

                /** MapTravelPlan expires_at_unix_ms */
                expires_at_unix_ms?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MapTravelPlan. */
            type $Shape = minimir.v1.MapTravelPlan.$Properties;
        }

        /**
         * Properties of a MapTravelCommit.
         * @deprecated Use minimir.v1.MapTravelCommit.$Properties instead.
         */
        interface IMapTravelCommit extends minimir.v1.MapTravelCommit.$Properties {
        }

        /** Represents a MapTravelCommit. */
        class MapTravelCommit {

            /**
             * Constructs a new MapTravelCommit.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.MapTravelCommit.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MapTravelCommit scene_epoch. */
            scene_epoch: Long;

            /** MapTravelCommit travel_id. */
            travel_id: string;

            /** MapTravelCommit cancel. */
            cancel: boolean;

            /**
             * Creates a new MapTravelCommit instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MapTravelCommit instance
             */
            static create(properties: minimir.v1.MapTravelCommit.$Shape): minimir.v1.MapTravelCommit & minimir.v1.MapTravelCommit.$Shape;
            static create(properties?: minimir.v1.MapTravelCommit.$Properties): minimir.v1.MapTravelCommit;

            /**
             * Encodes the specified MapTravelCommit message. Does not implicitly {@link minimir.v1.MapTravelCommit.verify|verify} messages.
             * @param message MapTravelCommit message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.MapTravelCommit.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MapTravelCommit message, length delimited. Does not implicitly {@link minimir.v1.MapTravelCommit.verify|verify} messages.
             * @param message MapTravelCommit message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.MapTravelCommit.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MapTravelCommit message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.MapTravelCommit & minimir.v1.MapTravelCommit.$Shape} MapTravelCommit
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.MapTravelCommit & minimir.v1.MapTravelCommit.$Shape;

            /**
             * Decodes a MapTravelCommit message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.MapTravelCommit & minimir.v1.MapTravelCommit.$Shape} MapTravelCommit
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.MapTravelCommit & minimir.v1.MapTravelCommit.$Shape;

            /**
             * Verifies a MapTravelCommit message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for MapTravelCommit
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MapTravelCommit {

            /** Properties of a MapTravelCommit. */
            interface $Properties {

                /** MapTravelCommit scene_epoch */
                scene_epoch?: (Long|null);

                /** MapTravelCommit travel_id */
                travel_id?: (string|null);

                /** MapTravelCommit cancel */
                cancel?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MapTravelCommit. */
            type $Shape = minimir.v1.MapTravelCommit.$Properties;
        }

        /**
         * Properties of a MapTravelState.
         * @deprecated Use minimir.v1.MapTravelState.$Properties instead.
         */
        interface IMapTravelState extends minimir.v1.MapTravelState.$Properties {
        }

        /** Represents a MapTravelState. */
        class MapTravelState {

            /**
             * Constructs a new MapTravelState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.MapTravelState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MapTravelState travel_id. */
            travel_id: string;

            /** MapTravelState from_scene_epoch. */
            from_scene_epoch: Long;

            /** MapTravelState scene_epoch. */
            scene_epoch: Long;

            /** MapTravelState phase. */
            phase: number;

            /** MapTravelState target_map_id. */
            target_map_id: Long;

            /**
             * Creates a new MapTravelState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MapTravelState instance
             */
            static create(properties: minimir.v1.MapTravelState.$Shape): minimir.v1.MapTravelState & minimir.v1.MapTravelState.$Shape;
            static create(properties?: minimir.v1.MapTravelState.$Properties): minimir.v1.MapTravelState;

            /**
             * Encodes the specified MapTravelState message. Does not implicitly {@link minimir.v1.MapTravelState.verify|verify} messages.
             * @param message MapTravelState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.MapTravelState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MapTravelState message, length delimited. Does not implicitly {@link minimir.v1.MapTravelState.verify|verify} messages.
             * @param message MapTravelState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.MapTravelState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MapTravelState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.MapTravelState & minimir.v1.MapTravelState.$Shape} MapTravelState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.MapTravelState & minimir.v1.MapTravelState.$Shape;

            /**
             * Decodes a MapTravelState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.MapTravelState & minimir.v1.MapTravelState.$Shape} MapTravelState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.MapTravelState & minimir.v1.MapTravelState.$Shape;

            /**
             * Verifies a MapTravelState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for MapTravelState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MapTravelState {

            /** Properties of a MapTravelState. */
            interface $Properties {

                /** MapTravelState travel_id */
                travel_id?: (string|null);

                /** MapTravelState from_scene_epoch */
                from_scene_epoch?: (Long|null);

                /** MapTravelState scene_epoch */
                scene_epoch?: (Long|null);

                /** MapTravelState phase */
                phase?: (number|null);

                /** MapTravelState target_map_id */
                target_map_id?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MapTravelState. */
            type $Shape = minimir.v1.MapTravelState.$Properties;
        }

        /**
         * Properties of a SyncReady.
         * @deprecated Use minimir.v1.SyncReady.$Properties instead.
         */
        interface ISyncReady extends minimir.v1.SyncReady.$Properties {
        }

        /** Represents a SyncReady. */
        class SyncReady {

            /**
             * Constructs a new SyncReady.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SyncReady.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SyncReady scene_epoch. */
            scene_epoch: Long;

            /** SyncReady baseline_id. */
            baseline_id: Long;

            /** SyncReady next_economic_command_seq. */
            next_economic_command_seq: Long;

            /**
             * Creates a new SyncReady instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SyncReady instance
             */
            static create(properties: minimir.v1.SyncReady.$Shape): minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape;
            static create(properties?: minimir.v1.SyncReady.$Properties): minimir.v1.SyncReady;

            /**
             * Encodes the specified SyncReady message. Does not implicitly {@link minimir.v1.SyncReady.verify|verify} messages.
             * @param message SyncReady message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SyncReady.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SyncReady message, length delimited. Does not implicitly {@link minimir.v1.SyncReady.verify|verify} messages.
             * @param message SyncReady message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SyncReady.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SyncReady message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape} SyncReady
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape;

            /**
             * Decodes a SyncReady message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape} SyncReady
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape;

            /**
             * Verifies a SyncReady message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SyncReady
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SyncReady {

            /** Properties of a SyncReady. */
            interface $Properties {

                /** SyncReady scene_epoch */
                scene_epoch?: (Long|null);

                /** SyncReady baseline_id */
                baseline_id?: (Long|null);

                /** SyncReady next_economic_command_seq */
                next_economic_command_seq?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SyncReady. */
            type $Shape = minimir.v1.SyncReady.$Properties;
        }

        /**
         * Properties of a RoleBindingPoint.
         * @deprecated Use minimir.v1.RoleBindingPoint.$Properties instead.
         */
        interface IRoleBindingPoint extends minimir.v1.RoleBindingPoint.$Properties {
        }

        /** Represents a RoleBindingPoint. */
        class RoleBindingPoint {

            /**
             * Constructs a new RoleBindingPoint.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RoleBindingPoint.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RoleBindingPoint map_id. */
            map_id: Long;

            /** RoleBindingPoint center. */
            center?: (minimir.v1.GridPoint.$Properties|null);

            /**
             * Creates a new RoleBindingPoint instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RoleBindingPoint instance
             */
            static create(properties: minimir.v1.RoleBindingPoint.$Shape): minimir.v1.RoleBindingPoint & minimir.v1.RoleBindingPoint.$Shape;
            static create(properties?: minimir.v1.RoleBindingPoint.$Properties): minimir.v1.RoleBindingPoint;

            /**
             * Encodes the specified RoleBindingPoint message. Does not implicitly {@link minimir.v1.RoleBindingPoint.verify|verify} messages.
             * @param message RoleBindingPoint message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RoleBindingPoint.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RoleBindingPoint message, length delimited. Does not implicitly {@link minimir.v1.RoleBindingPoint.verify|verify} messages.
             * @param message RoleBindingPoint message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RoleBindingPoint.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RoleBindingPoint message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RoleBindingPoint & minimir.v1.RoleBindingPoint.$Shape} RoleBindingPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RoleBindingPoint & minimir.v1.RoleBindingPoint.$Shape;

            /**
             * Decodes a RoleBindingPoint message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleBindingPoint & minimir.v1.RoleBindingPoint.$Shape} RoleBindingPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RoleBindingPoint & minimir.v1.RoleBindingPoint.$Shape;

            /**
             * Verifies a RoleBindingPoint message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RoleBindingPoint
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RoleBindingPoint {

            /** Properties of a RoleBindingPoint. */
            interface $Properties {

                /** RoleBindingPoint map_id */
                map_id?: (Long|null);

                /** RoleBindingPoint center */
                center?: (minimir.v1.GridPoint.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RoleBindingPoint. */
            type $Shape = minimir.v1.RoleBindingPoint.$Properties;
        }

        /**
         * Properties of a BindingState.
         * @deprecated Use minimir.v1.BindingState.$Properties instead.
         */
        interface IBindingState extends minimir.v1.BindingState.$Properties {
        }

        /** Represents a BindingState. */
        class BindingState {

            /**
             * Constructs a new BindingState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.BindingState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** BindingState role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** BindingState scene_epoch. */
            scene_epoch: Long;

            /** BindingState role_revision. */
            role_revision: Long;

            /** BindingState source_seq. */
            source_seq: Long;

            /** BindingState binding. */
            binding?: (minimir.v1.RoleBindingPoint.$Properties|null);

            /** BindingState cause. */
            cause: number;

            /** BindingState safe_zone_id. */
            safe_zone_id: Long;

            /** BindingState trusted. */
            trusted: boolean;

            /**
             * Creates a new BindingState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns BindingState instance
             */
            static create(properties: minimir.v1.BindingState.$Shape): minimir.v1.BindingState & minimir.v1.BindingState.$Shape;
            static create(properties?: minimir.v1.BindingState.$Properties): minimir.v1.BindingState;

            /**
             * Encodes the specified BindingState message. Does not implicitly {@link minimir.v1.BindingState.verify|verify} messages.
             * @param message BindingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.BindingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified BindingState message, length delimited. Does not implicitly {@link minimir.v1.BindingState.verify|verify} messages.
             * @param message BindingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.BindingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a BindingState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.BindingState & minimir.v1.BindingState.$Shape} BindingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.BindingState & minimir.v1.BindingState.$Shape;

            /**
             * Decodes a BindingState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.BindingState & minimir.v1.BindingState.$Shape} BindingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.BindingState & minimir.v1.BindingState.$Shape;

            /**
             * Verifies a BindingState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for BindingState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace BindingState {

            /** Properties of a BindingState. */
            interface $Properties {

                /** BindingState role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** BindingState scene_epoch */
                scene_epoch?: (Long|null);

                /** BindingState role_revision */
                role_revision?: (Long|null);

                /** BindingState source_seq */
                source_seq?: (Long|null);

                /** BindingState binding */
                binding?: (minimir.v1.RoleBindingPoint.$Properties|null);

                /** BindingState cause */
                cause?: (number|null);

                /** BindingState safe_zone_id */
                safe_zone_id?: (Long|null);

                /** BindingState trusted */
                trusted?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a BindingState. */
            type $Shape = minimir.v1.BindingState.$Properties;
        }

        /**
         * Properties of a TeleportResult.
         * @deprecated Use minimir.v1.TeleportResult.$Properties instead.
         */
        interface ITeleportResult extends minimir.v1.TeleportResult.$Properties {
        }

        /** Represents a TeleportResult. */
        class TeleportResult {

            /**
             * Constructs a new TeleportResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.TeleportResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** TeleportResult role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** TeleportResult from_scene_epoch. */
            from_scene_epoch: Long;

            /** TeleportResult scene_epoch. */
            scene_epoch: Long;

            /** TeleportResult cast_id. */
            cast_id: Long;

            /** TeleportResult player_life. */
            player_life: Long;

            /** TeleportResult target_map_id. */
            target_map_id: Long;

            /** TeleportResult position. */
            position?: (minimir.v1.GridPoint.$Properties|null);

            /** TeleportResult movement_revision. */
            movement_revision: Long;

            /** TeleportResult outcome. */
            outcome: number;

            /** TeleportResult transition_id. */
            transition_id: string;

            /**
             * Creates a new TeleportResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns TeleportResult instance
             */
            static create(properties: minimir.v1.TeleportResult.$Shape): minimir.v1.TeleportResult & minimir.v1.TeleportResult.$Shape;
            static create(properties?: minimir.v1.TeleportResult.$Properties): minimir.v1.TeleportResult;

            /**
             * Encodes the specified TeleportResult message. Does not implicitly {@link minimir.v1.TeleportResult.verify|verify} messages.
             * @param message TeleportResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.TeleportResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified TeleportResult message, length delimited. Does not implicitly {@link minimir.v1.TeleportResult.verify|verify} messages.
             * @param message TeleportResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.TeleportResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a TeleportResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.TeleportResult & minimir.v1.TeleportResult.$Shape} TeleportResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.TeleportResult & minimir.v1.TeleportResult.$Shape;

            /**
             * Decodes a TeleportResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.TeleportResult & minimir.v1.TeleportResult.$Shape} TeleportResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.TeleportResult & minimir.v1.TeleportResult.$Shape;

            /**
             * Verifies a TeleportResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for TeleportResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace TeleportResult {

            /** Properties of a TeleportResult. */
            interface $Properties {

                /** TeleportResult role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** TeleportResult from_scene_epoch */
                from_scene_epoch?: (Long|null);

                /** TeleportResult scene_epoch */
                scene_epoch?: (Long|null);

                /** TeleportResult cast_id */
                cast_id?: (Long|null);

                /** TeleportResult player_life */
                player_life?: (Long|null);

                /** TeleportResult target_map_id */
                target_map_id?: (Long|null);

                /** TeleportResult position */
                position?: (minimir.v1.GridPoint.$Properties|null);

                /** TeleportResult movement_revision */
                movement_revision?: (Long|null);

                /** TeleportResult outcome */
                outcome?: (number|null);

                /** TeleportResult transition_id */
                transition_id?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a TeleportResult. */
            type $Shape = minimir.v1.TeleportResult.$Properties;
        }

        /**
         * Properties of a PetTrainingState.
         * @deprecated Use minimir.v1.PetTrainingState.$Properties instead.
         */
        interface IPetTrainingState extends minimir.v1.PetTrainingState.$Properties {
        }

        /** Represents a PetTrainingState. */
        class PetTrainingState {

            /**
             * Constructs a new PetTrainingState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.PetTrainingState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** PetTrainingState training_level. */
            training_level: number;

            /** PetTrainingState max_training_level. */
            max_training_level: number;

            /** PetTrainingState experience. */
            experience: Long;

            /** PetTrainingState next_level_experience. */
            next_level_experience: Long;

            /**
             * Creates a new PetTrainingState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns PetTrainingState instance
             */
            static create(properties: minimir.v1.PetTrainingState.$Shape): minimir.v1.PetTrainingState & minimir.v1.PetTrainingState.$Shape;
            static create(properties?: minimir.v1.PetTrainingState.$Properties): minimir.v1.PetTrainingState;

            /**
             * Encodes the specified PetTrainingState message. Does not implicitly {@link minimir.v1.PetTrainingState.verify|verify} messages.
             * @param message PetTrainingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.PetTrainingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified PetTrainingState message, length delimited. Does not implicitly {@link minimir.v1.PetTrainingState.verify|verify} messages.
             * @param message PetTrainingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.PetTrainingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a PetTrainingState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.PetTrainingState & minimir.v1.PetTrainingState.$Shape} PetTrainingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.PetTrainingState & minimir.v1.PetTrainingState.$Shape;

            /**
             * Decodes a PetTrainingState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.PetTrainingState & minimir.v1.PetTrainingState.$Shape} PetTrainingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.PetTrainingState & minimir.v1.PetTrainingState.$Shape;

            /**
             * Verifies a PetTrainingState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for PetTrainingState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace PetTrainingState {

            /** Properties of a PetTrainingState. */
            interface $Properties {

                /** PetTrainingState training_level */
                training_level?: (number|null);

                /** PetTrainingState max_training_level */
                max_training_level?: (number|null);

                /** PetTrainingState experience */
                experience?: (Long|null);

                /** PetTrainingState next_level_experience */
                next_level_experience?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a PetTrainingState. */
            type $Shape = minimir.v1.PetTrainingState.$Properties;
        }

        /**
         * Properties of an OwnedPetState.
         * @deprecated Use minimir.v1.OwnedPetState.$Properties instead.
         */
        interface IOwnedPetState extends minimir.v1.OwnedPetState.$Properties {
        }

        /** Represents an OwnedPetState. */
        class OwnedPetState {

            /**
             * Constructs a new OwnedPetState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.OwnedPetState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** OwnedPetState entity_handle. */
            entity_handle: number;

            /** OwnedPetState life. */
            life: Long;

            /** OwnedPetState definition_id. */
            definition_id: number;

            /** OwnedPetState map_id. */
            map_id: Long;

            /** OwnedPetState hp. */
            hp: Long;

            /** OwnedPetState hp_max. */
            hp_max: Long;

            /** OwnedPetState pet_training. */
            pet_training?: (minimir.v1.PetTrainingState.$Properties|null);

            /** OwnedPetState recall_skill_ids. */
            recall_skill_ids: number[];

            /** OwnedPetState follow_suppressed. */
            follow_suppressed: boolean;

            /**
             * Creates a new OwnedPetState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns OwnedPetState instance
             */
            static create(properties: minimir.v1.OwnedPetState.$Shape): minimir.v1.OwnedPetState & minimir.v1.OwnedPetState.$Shape;
            static create(properties?: minimir.v1.OwnedPetState.$Properties): minimir.v1.OwnedPetState;

            /**
             * Encodes the specified OwnedPetState message. Does not implicitly {@link minimir.v1.OwnedPetState.verify|verify} messages.
             * @param message OwnedPetState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.OwnedPetState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified OwnedPetState message, length delimited. Does not implicitly {@link minimir.v1.OwnedPetState.verify|verify} messages.
             * @param message OwnedPetState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.OwnedPetState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an OwnedPetState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.OwnedPetState & minimir.v1.OwnedPetState.$Shape} OwnedPetState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.OwnedPetState & minimir.v1.OwnedPetState.$Shape;

            /**
             * Decodes an OwnedPetState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.OwnedPetState & minimir.v1.OwnedPetState.$Shape} OwnedPetState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.OwnedPetState & minimir.v1.OwnedPetState.$Shape;

            /**
             * Verifies an OwnedPetState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for OwnedPetState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace OwnedPetState {

            /** Properties of an OwnedPetState. */
            interface $Properties {

                /** OwnedPetState entity_handle */
                entity_handle?: (number|null);

                /** OwnedPetState life */
                life?: (Long|null);

                /** OwnedPetState definition_id */
                definition_id?: (number|null);

                /** OwnedPetState map_id */
                map_id?: (Long|null);

                /** OwnedPetState hp */
                hp?: (Long|null);

                /** OwnedPetState hp_max */
                hp_max?: (Long|null);

                /** OwnedPetState pet_training */
                pet_training?: (minimir.v1.PetTrainingState.$Properties|null);

                /** OwnedPetState recall_skill_ids */
                recall_skill_ids?: (number[]|null);

                /** OwnedPetState follow_suppressed */
                follow_suppressed?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an OwnedPetState. */
            type $Shape = minimir.v1.OwnedPetState.$Properties;
        }

        /**
         * Properties of a PetBorn.
         * @deprecated Use minimir.v1.PetBorn.$Properties instead.
         */
        interface IPetBorn extends minimir.v1.PetBorn.$Properties {
        }

        /** Represents a PetBorn. */
        class PetBorn {

            /**
             * Constructs a new PetBorn.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.PetBorn.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** PetBorn scene_epoch. */
            scene_epoch: Long;

            /** PetBorn baseline_id. */
            baseline_id: Long;

            /** PetBorn state_sequence. */
            state_sequence: Long;

            /** PetBorn entity_handle. */
            entity_handle: number;

            /** PetBorn entity_life. */
            entity_life: Long;

            /** PetBorn definition_id. */
            definition_id: number;

            /** PetBorn map_id. */
            map_id: Long;

            /** PetBorn source_cast_id. */
            source_cast_id: Long;

            /** PetBorn source_skill_id. */
            source_skill_id: number;

            /** PetBorn born_at_unix_ms. */
            born_at_unix_ms: Long;

            /** PetBorn movement_revision. */
            movement_revision: Long;

            /** PetBorn direction. */
            direction: number;

            /** PetBorn role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** PetBorn elapsed_ms. */
            elapsed_ms: number;

            /**
             * Creates a new PetBorn instance using the specified properties.
             * @param [properties] Properties to set
             * @returns PetBorn instance
             */
            static create(properties: minimir.v1.PetBorn.$Shape): minimir.v1.PetBorn & minimir.v1.PetBorn.$Shape;
            static create(properties?: minimir.v1.PetBorn.$Properties): minimir.v1.PetBorn;

            /**
             * Encodes the specified PetBorn message. Does not implicitly {@link minimir.v1.PetBorn.verify|verify} messages.
             * @param message PetBorn message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.PetBorn.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified PetBorn message, length delimited. Does not implicitly {@link minimir.v1.PetBorn.verify|verify} messages.
             * @param message PetBorn message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.PetBorn.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a PetBorn message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.PetBorn & minimir.v1.PetBorn.$Shape} PetBorn
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.PetBorn & minimir.v1.PetBorn.$Shape;

            /**
             * Decodes a PetBorn message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.PetBorn & minimir.v1.PetBorn.$Shape} PetBorn
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.PetBorn & minimir.v1.PetBorn.$Shape;

            /**
             * Verifies a PetBorn message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for PetBorn
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace PetBorn {

            /** Properties of a PetBorn. */
            interface $Properties {

                /** PetBorn scene_epoch */
                scene_epoch?: (Long|null);

                /** PetBorn baseline_id */
                baseline_id?: (Long|null);

                /** PetBorn state_sequence */
                state_sequence?: (Long|null);

                /** PetBorn entity_handle */
                entity_handle?: (number|null);

                /** PetBorn entity_life */
                entity_life?: (Long|null);

                /** PetBorn definition_id */
                definition_id?: (number|null);

                /** PetBorn map_id */
                map_id?: (Long|null);

                /** PetBorn source_cast_id */
                source_cast_id?: (Long|null);

                /** PetBorn source_skill_id */
                source_skill_id?: (number|null);

                /** PetBorn born_at_unix_ms */
                born_at_unix_ms?: (Long|null);

                /** PetBorn movement_revision */
                movement_revision?: (Long|null);

                /** PetBorn direction */
                direction?: (number|null);

                /** PetBorn role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** PetBorn elapsed_ms */
                elapsed_ms?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a PetBorn. */
            type $Shape = minimir.v1.PetBorn.$Properties;
        }

        /**
         * Properties of an EntityState.
         * @deprecated Use minimir.v1.EntityState.$Properties instead.
         */
        interface IEntityState extends minimir.v1.EntityState.$Properties {
        }

        /** Represents an EntityState. */
        class EntityState {

            /**
             * Constructs a new EntityState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EntityState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EntityState entity_handle. */
            entity_handle: number;

            /** EntityState entity_kind. */
            entity_kind: number;

            /** EntityState definition_id. */
            definition_id: number;

            /** EntityState position. */
            position?: (minimir.v1.GridPoint.$Properties|null);

            /** EntityState direction. */
            direction: number;

            /** EntityState hp. */
            hp: Long;

            /** EntityState hp_max. */
            hp_max: Long;

            /** EntityState action. */
            action: number;

            /** EntityState mp. */
            mp: Long;

            /** EntityState mp_max. */
            mp_max: Long;

            /** EntityState move_interval_ms. */
            move_interval_ms: number;

            /** EntityState attack_sequence. */
            attack_sequence: number;

            /** EntityState cast_sequence. */
            cast_sequence: number;

            /** EntityState cast_skill_id. */
            cast_skill_id: number;

            /** EntityState life. */
            life: Long;

            /** EntityState attack_interval_ms. */
            attack_interval_ms: number;

            /** EntityState attack_skill_id. */
            attack_skill_id: number;

            /** EntityState attack_skill_level. */
            attack_skill_level: number;

            /** EntityState statuses. */
            statuses: minimir.v1.EntityStatusState.$Properties[];

            /** EntityState combat_attributes. */
            combat_attributes?: (minimir.v1.CombatAttributes.$Properties|null);

            /** EntityState movement_revision. */
            movement_revision: Long;

            /** EntityState movement_kind. */
            movement_kind: number;

            /** EntityState pet_training. */
            pet_training?: (minimir.v1.PetTrainingState.$Properties|null);

            /** EntityState is_pet_corpse. */
            is_pet_corpse: boolean;

            /**
             * Creates a new EntityState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EntityState instance
             */
            static create(properties: minimir.v1.EntityState.$Shape): minimir.v1.EntityState & minimir.v1.EntityState.$Shape;
            static create(properties?: minimir.v1.EntityState.$Properties): minimir.v1.EntityState;

            /**
             * Encodes the specified EntityState message. Does not implicitly {@link minimir.v1.EntityState.verify|verify} messages.
             * @param message EntityState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EntityState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EntityState message, length delimited. Does not implicitly {@link minimir.v1.EntityState.verify|verify} messages.
             * @param message EntityState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EntityState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EntityState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EntityState & minimir.v1.EntityState.$Shape} EntityState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EntityState & minimir.v1.EntityState.$Shape;

            /**
             * Decodes an EntityState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EntityState & minimir.v1.EntityState.$Shape} EntityState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EntityState & minimir.v1.EntityState.$Shape;

            /**
             * Verifies an EntityState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EntityState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EntityState {

            /** Properties of an EntityState. */
            interface $Properties {

                /** EntityState entity_handle */
                entity_handle?: (number|null);

                /** EntityState entity_kind */
                entity_kind?: (number|null);

                /** EntityState definition_id */
                definition_id?: (number|null);

                /** EntityState position */
                position?: (minimir.v1.GridPoint.$Properties|null);

                /** EntityState direction */
                direction?: (number|null);

                /** EntityState hp */
                hp?: (Long|null);

                /** EntityState hp_max */
                hp_max?: (Long|null);

                /** EntityState action */
                action?: (number|null);

                /** EntityState mp */
                mp?: (Long|null);

                /** EntityState mp_max */
                mp_max?: (Long|null);

                /** EntityState move_interval_ms */
                move_interval_ms?: (number|null);

                /** EntityState attack_sequence */
                attack_sequence?: (number|null);

                /** EntityState cast_sequence */
                cast_sequence?: (number|null);

                /** EntityState cast_skill_id */
                cast_skill_id?: (number|null);

                /** EntityState life */
                life?: (Long|null);

                /** EntityState attack_interval_ms */
                attack_interval_ms?: (number|null);

                /** EntityState attack_skill_id */
                attack_skill_id?: (number|null);

                /** EntityState attack_skill_level */
                attack_skill_level?: (number|null);

                /** EntityState statuses */
                statuses?: (minimir.v1.EntityStatusState.$Properties[]|null);

                /** EntityState combat_attributes */
                combat_attributes?: (minimir.v1.CombatAttributes.$Properties|null);

                /** EntityState movement_revision */
                movement_revision?: (Long|null);

                /** EntityState movement_kind */
                movement_kind?: (number|null);

                /** EntityState pet_training */
                pet_training?: (minimir.v1.PetTrainingState.$Properties|null);

                /** EntityState is_pet_corpse */
                is_pet_corpse?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntityState. */
            type $Shape = minimir.v1.EntityState.$Properties;
        }

        /**
         * Properties of an EntityDisplacement.
         * @deprecated Use minimir.v1.EntityDisplacement.$Properties instead.
         */
        interface IEntityDisplacement extends minimir.v1.EntityDisplacement.$Properties {
        }

        /** Represents an EntityDisplacement. */
        class EntityDisplacement {

            /**
             * Constructs a new EntityDisplacement.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EntityDisplacement.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EntityDisplacement scene_epoch. */
            scene_epoch: Long;

            /** EntityDisplacement entity_handle. */
            entity_handle: number;

            /** EntityDisplacement entity_life. */
            entity_life: Long;

            /** EntityDisplacement movement_revision. */
            movement_revision: Long;

            /** EntityDisplacement kind. */
            kind: number;

            /** EntityDisplacement origin. */
            origin?: (minimir.v1.GridPoint.$Properties|null);

            /** EntityDisplacement path. */
            path: minimir.v1.GridPoint.$Properties[];

            /** EntityDisplacement facing_direction. */
            facing_direction: number;

            /** EntityDisplacement movement_direction. */
            movement_direction: number;

            /** EntityDisplacement step_duration_ms. */
            step_duration_ms: number;

            /** EntityDisplacement source_skill_id. */
            source_skill_id: number;

            /** EntityDisplacement cast_id. */
            cast_id: Long;

            /** EntityDisplacement attack_sequence. */
            attack_sequence: number;

            /** EntityDisplacement cast_sequence. */
            cast_sequence: number;

            /**
             * Creates a new EntityDisplacement instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EntityDisplacement instance
             */
            static create(properties: minimir.v1.EntityDisplacement.$Shape): minimir.v1.EntityDisplacement & minimir.v1.EntityDisplacement.$Shape;
            static create(properties?: minimir.v1.EntityDisplacement.$Properties): minimir.v1.EntityDisplacement;

            /**
             * Encodes the specified EntityDisplacement message. Does not implicitly {@link minimir.v1.EntityDisplacement.verify|verify} messages.
             * @param message EntityDisplacement message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EntityDisplacement.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EntityDisplacement message, length delimited. Does not implicitly {@link minimir.v1.EntityDisplacement.verify|verify} messages.
             * @param message EntityDisplacement message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EntityDisplacement.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EntityDisplacement message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EntityDisplacement & minimir.v1.EntityDisplacement.$Shape} EntityDisplacement
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EntityDisplacement & minimir.v1.EntityDisplacement.$Shape;

            /**
             * Decodes an EntityDisplacement message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EntityDisplacement & minimir.v1.EntityDisplacement.$Shape} EntityDisplacement
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EntityDisplacement & minimir.v1.EntityDisplacement.$Shape;

            /**
             * Verifies an EntityDisplacement message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EntityDisplacement
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EntityDisplacement {

            /** Properties of an EntityDisplacement. */
            interface $Properties {

                /** EntityDisplacement scene_epoch */
                scene_epoch?: (Long|null);

                /** EntityDisplacement entity_handle */
                entity_handle?: (number|null);

                /** EntityDisplacement entity_life */
                entity_life?: (Long|null);

                /** EntityDisplacement movement_revision */
                movement_revision?: (Long|null);

                /** EntityDisplacement kind */
                kind?: (number|null);

                /** EntityDisplacement origin */
                origin?: (minimir.v1.GridPoint.$Properties|null);

                /** EntityDisplacement path */
                path?: (minimir.v1.GridPoint.$Properties[]|null);

                /** EntityDisplacement facing_direction */
                facing_direction?: (number|null);

                /** EntityDisplacement movement_direction */
                movement_direction?: (number|null);

                /** EntityDisplacement step_duration_ms */
                step_duration_ms?: (number|null);

                /** EntityDisplacement source_skill_id */
                source_skill_id?: (number|null);

                /** EntityDisplacement cast_id */
                cast_id?: (Long|null);

                /** EntityDisplacement attack_sequence */
                attack_sequence?: (number|null);

                /** EntityDisplacement cast_sequence */
                cast_sequence?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntityDisplacement. */
            type $Shape = minimir.v1.EntityDisplacement.$Properties;
        }

        /**
         * Properties of a NavigationState.
         * @deprecated Use minimir.v1.NavigationState.$Properties instead.
         */
        interface INavigationState extends minimir.v1.NavigationState.$Properties {
        }

        /** Represents a NavigationState. */
        class NavigationState {

            /**
             * Constructs a new NavigationState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.NavigationState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** NavigationState input_sequence. */
            input_sequence: number;

            /** NavigationState destination. */
            destination?: (minimir.v1.GridPoint.$Properties|null);

            /** NavigationState status. */
            status: number;

            /** NavigationState revision. */
            revision: Long;

            /**
             * Creates a new NavigationState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns NavigationState instance
             */
            static create(properties: minimir.v1.NavigationState.$Shape): minimir.v1.NavigationState & minimir.v1.NavigationState.$Shape;
            static create(properties?: minimir.v1.NavigationState.$Properties): minimir.v1.NavigationState;

            /**
             * Encodes the specified NavigationState message. Does not implicitly {@link minimir.v1.NavigationState.verify|verify} messages.
             * @param message NavigationState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.NavigationState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified NavigationState message, length delimited. Does not implicitly {@link minimir.v1.NavigationState.verify|verify} messages.
             * @param message NavigationState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.NavigationState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a NavigationState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.NavigationState & minimir.v1.NavigationState.$Shape} NavigationState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.NavigationState & minimir.v1.NavigationState.$Shape;

            /**
             * Decodes a NavigationState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.NavigationState & minimir.v1.NavigationState.$Shape} NavigationState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.NavigationState & minimir.v1.NavigationState.$Shape;

            /**
             * Verifies a NavigationState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for NavigationState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace NavigationState {

            /** Properties of a NavigationState. */
            interface $Properties {

                /** NavigationState input_sequence */
                input_sequence?: (number|null);

                /** NavigationState destination */
                destination?: (minimir.v1.GridPoint.$Properties|null);

                /** NavigationState status */
                status?: (number|null);

                /** NavigationState revision */
                revision?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a NavigationState. */
            type $Shape = minimir.v1.NavigationState.$Properties;
        }

        /**
         * Properties of an EntityStatusState.
         * @deprecated Use minimir.v1.EntityStatusState.$Properties instead.
         */
        interface IEntityStatusState extends minimir.v1.EntityStatusState.$Properties {
        }

        /** Represents an EntityStatusState. */
        class EntityStatusState {

            /**
             * Constructs a new EntityStatusState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EntityStatusState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EntityStatusState status_id. */
            status_id: number;

            /** EntityStatusState source_skill_id. */
            source_skill_id: number;

            /** EntityStatusState skill_level. */
            skill_level: number;

            /** EntityStatusState remaining_ms. */
            remaining_ms: number;

            /** EntityStatusState max_physical_defence_bonus. */
            max_physical_defence_bonus: Long;

            /** EntityStatusState max_magic_defence_bonus. */
            max_magic_defence_bonus: Long;

            /** EntityStatusState damage_reduction_percent. */
            damage_reduction_percent: number;

            /** EntityStatusState caster_handle. */
            caster_handle: number;

            /** EntityStatusState caster_life. */
            caster_life: Long;

            /** EntityStatusState teleport_mana_penalty_percent. */
            teleport_mana_penalty_percent: number;

            /**
             * Creates a new EntityStatusState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EntityStatusState instance
             */
            static create(properties: minimir.v1.EntityStatusState.$Shape): minimir.v1.EntityStatusState & minimir.v1.EntityStatusState.$Shape;
            static create(properties?: minimir.v1.EntityStatusState.$Properties): minimir.v1.EntityStatusState;

            /**
             * Encodes the specified EntityStatusState message. Does not implicitly {@link minimir.v1.EntityStatusState.verify|verify} messages.
             * @param message EntityStatusState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EntityStatusState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EntityStatusState message, length delimited. Does not implicitly {@link minimir.v1.EntityStatusState.verify|verify} messages.
             * @param message EntityStatusState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EntityStatusState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EntityStatusState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EntityStatusState & minimir.v1.EntityStatusState.$Shape} EntityStatusState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EntityStatusState & minimir.v1.EntityStatusState.$Shape;

            /**
             * Decodes an EntityStatusState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EntityStatusState & minimir.v1.EntityStatusState.$Shape} EntityStatusState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EntityStatusState & minimir.v1.EntityStatusState.$Shape;

            /**
             * Verifies an EntityStatusState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EntityStatusState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EntityStatusState {

            /** Properties of an EntityStatusState. */
            interface $Properties {

                /** EntityStatusState status_id */
                status_id?: (number|null);

                /** EntityStatusState source_skill_id */
                source_skill_id?: (number|null);

                /** EntityStatusState skill_level */
                skill_level?: (number|null);

                /** EntityStatusState remaining_ms */
                remaining_ms?: (number|null);

                /** EntityStatusState max_physical_defence_bonus */
                max_physical_defence_bonus?: (Long|null);

                /** EntityStatusState max_magic_defence_bonus */
                max_magic_defence_bonus?: (Long|null);

                /** EntityStatusState damage_reduction_percent */
                damage_reduction_percent?: (number|null);

                /** EntityStatusState caster_handle */
                caster_handle?: (number|null);

                /** EntityStatusState caster_life */
                caster_life?: (Long|null);

                /** EntityStatusState teleport_mana_penalty_percent */
                teleport_mana_penalty_percent?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntityStatusState. */
            type $Shape = minimir.v1.EntityStatusState.$Properties;
        }

        /**
         * Properties of a CombatAttributes.
         * @deprecated Use minimir.v1.CombatAttributes.$Properties instead.
         */
        interface ICombatAttributes extends minimir.v1.CombatAttributes.$Properties {
        }

        /** Represents a CombatAttributes. */
        class CombatAttributes {

            /**
             * Constructs a new CombatAttributes.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CombatAttributes.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CombatAttributes physical_attack_min. */
            physical_attack_min: Long;

            /** CombatAttributes physical_attack_max. */
            physical_attack_max: Long;

            /** CombatAttributes physical_defence_min. */
            physical_defence_min: Long;

            /** CombatAttributes physical_defence_max. */
            physical_defence_max: Long;

            /** CombatAttributes magic_defence_min. */
            magic_defence_min: Long;

            /** CombatAttributes magic_defence_max. */
            magic_defence_max: Long;

            /** CombatAttributes magic_attack_min. */
            magic_attack_min: Long;

            /** CombatAttributes magic_attack_max. */
            magic_attack_max: Long;

            /** CombatAttributes spirit_attack_min. */
            spirit_attack_min: Long;

            /** CombatAttributes spirit_attack_max. */
            spirit_attack_max: Long;

            /** CombatAttributes accuracy. */
            accuracy: Long;

            /** CombatAttributes agility. */
            agility: Long;

            /** CombatAttributes damage_reduction_percent. */
            damage_reduction_percent: number;

            /**
             * Creates a new CombatAttributes instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CombatAttributes instance
             */
            static create(properties: minimir.v1.CombatAttributes.$Shape): minimir.v1.CombatAttributes & minimir.v1.CombatAttributes.$Shape;
            static create(properties?: minimir.v1.CombatAttributes.$Properties): minimir.v1.CombatAttributes;

            /**
             * Encodes the specified CombatAttributes message. Does not implicitly {@link minimir.v1.CombatAttributes.verify|verify} messages.
             * @param message CombatAttributes message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CombatAttributes.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CombatAttributes message, length delimited. Does not implicitly {@link minimir.v1.CombatAttributes.verify|verify} messages.
             * @param message CombatAttributes message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CombatAttributes.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CombatAttributes message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CombatAttributes & minimir.v1.CombatAttributes.$Shape} CombatAttributes
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CombatAttributes & minimir.v1.CombatAttributes.$Shape;

            /**
             * Decodes a CombatAttributes message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CombatAttributes & minimir.v1.CombatAttributes.$Shape} CombatAttributes
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CombatAttributes & minimir.v1.CombatAttributes.$Shape;

            /**
             * Verifies a CombatAttributes message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CombatAttributes
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CombatAttributes {

            /** Properties of a CombatAttributes. */
            interface $Properties {

                /** CombatAttributes physical_attack_min */
                physical_attack_min?: (Long|null);

                /** CombatAttributes physical_attack_max */
                physical_attack_max?: (Long|null);

                /** CombatAttributes physical_defence_min */
                physical_defence_min?: (Long|null);

                /** CombatAttributes physical_defence_max */
                physical_defence_max?: (Long|null);

                /** CombatAttributes magic_defence_min */
                magic_defence_min?: (Long|null);

                /** CombatAttributes magic_defence_max */
                magic_defence_max?: (Long|null);

                /** CombatAttributes magic_attack_min */
                magic_attack_min?: (Long|null);

                /** CombatAttributes magic_attack_max */
                magic_attack_max?: (Long|null);

                /** CombatAttributes spirit_attack_min */
                spirit_attack_min?: (Long|null);

                /** CombatAttributes spirit_attack_max */
                spirit_attack_max?: (Long|null);

                /** CombatAttributes accuracy */
                accuracy?: (Long|null);

                /** CombatAttributes agility */
                agility?: (Long|null);

                /** CombatAttributes damage_reduction_percent */
                damage_reduction_percent?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CombatAttributes. */
            type $Shape = minimir.v1.CombatAttributes.$Properties;
        }

        /**
         * Properties of a SceneBaselineChunk.
         * @deprecated Use minimir.v1.SceneBaselineChunk.$Properties instead.
         */
        interface ISceneBaselineChunk extends minimir.v1.SceneBaselineChunk.$Properties {
        }

        /** Represents a SceneBaselineChunk. */
        class SceneBaselineChunk {

            /**
             * Constructs a new SceneBaselineChunk.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SceneBaselineChunk.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SceneBaselineChunk scene_epoch. */
            scene_epoch: Long;

            /** SceneBaselineChunk baseline_id. */
            baseline_id: Long;

            /** SceneBaselineChunk chunk_index. */
            chunk_index: number;

            /** SceneBaselineChunk chunk_count. */
            chunk_count: number;

            /** SceneBaselineChunk state_sequence. */
            state_sequence: Long;

            /** SceneBaselineChunk entities. */
            entities: minimir.v1.EntityState.$Properties[];

            /** SceneBaselineChunk skill_mode_state. */
            skill_mode_state?: (minimir.v1.SkillModeState.$Properties|null);

            /** SceneBaselineChunk navigation_state. */
            navigation_state?: (minimir.v1.NavigationState.$Properties|null);

            /** SceneBaselineChunk safe_zone_id. */
            safe_zone_id: Long;

            /** SceneBaselineChunk owned_pet. */
            owned_pet?: (minimir.v1.OwnedPetState.$Properties|null);

            /**
             * Creates a new SceneBaselineChunk instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SceneBaselineChunk instance
             */
            static create(properties: minimir.v1.SceneBaselineChunk.$Shape): minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape;
            static create(properties?: minimir.v1.SceneBaselineChunk.$Properties): minimir.v1.SceneBaselineChunk;

            /**
             * Encodes the specified SceneBaselineChunk message. Does not implicitly {@link minimir.v1.SceneBaselineChunk.verify|verify} messages.
             * @param message SceneBaselineChunk message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SceneBaselineChunk.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SceneBaselineChunk message, length delimited. Does not implicitly {@link minimir.v1.SceneBaselineChunk.verify|verify} messages.
             * @param message SceneBaselineChunk message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SceneBaselineChunk.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SceneBaselineChunk message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape} SceneBaselineChunk
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape;

            /**
             * Decodes a SceneBaselineChunk message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape} SceneBaselineChunk
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape;

            /**
             * Verifies a SceneBaselineChunk message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SceneBaselineChunk
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SceneBaselineChunk {

            /** Properties of a SceneBaselineChunk. */
            interface $Properties {

                /** SceneBaselineChunk scene_epoch */
                scene_epoch?: (Long|null);

                /** SceneBaselineChunk baseline_id */
                baseline_id?: (Long|null);

                /** SceneBaselineChunk chunk_index */
                chunk_index?: (number|null);

                /** SceneBaselineChunk chunk_count */
                chunk_count?: (number|null);

                /** SceneBaselineChunk state_sequence */
                state_sequence?: (Long|null);

                /** SceneBaselineChunk entities */
                entities?: (minimir.v1.EntityState.$Properties[]|null);

                /** SceneBaselineChunk skill_mode_state */
                skill_mode_state?: (minimir.v1.SkillModeState.$Properties|null);

                /** SceneBaselineChunk navigation_state */
                navigation_state?: (minimir.v1.NavigationState.$Properties|null);

                /** SceneBaselineChunk safe_zone_id */
                safe_zone_id?: (Long|null);

                /** SceneBaselineChunk owned_pet */
                owned_pet?: (minimir.v1.OwnedPetState.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SceneBaselineChunk. */
            type $Shape = minimir.v1.SceneBaselineChunk.$Properties;
        }

        /**
         * Properties of a MoveIntent.
         * @deprecated Use minimir.v1.MoveIntent.$Properties instead.
         */
        interface IMoveIntent extends minimir.v1.MoveIntent.$Properties {
        }

        /** Represents a MoveIntent. */
        class MoveIntent {

            /**
             * Constructs a new MoveIntent.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.MoveIntent.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MoveIntent scene_epoch. */
            scene_epoch: Long;

            /** MoveIntent input_sequence. */
            input_sequence: number;

            /** MoveIntent destination. */
            destination?: (minimir.v1.GridPoint.$Properties|null);

            /** MoveIntent direction. */
            direction: number;

            /** MoveIntent release_joystick. */
            release_joystick: boolean;

            /** MoveIntent cancel_navigation. */
            cancel_navigation: boolean;

            /**
             * Creates a new MoveIntent instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MoveIntent instance
             */
            static create(properties: minimir.v1.MoveIntent.$Shape): minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape;
            static create(properties?: minimir.v1.MoveIntent.$Properties): minimir.v1.MoveIntent;

            /**
             * Encodes the specified MoveIntent message. Does not implicitly {@link minimir.v1.MoveIntent.verify|verify} messages.
             * @param message MoveIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.MoveIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MoveIntent message, length delimited. Does not implicitly {@link minimir.v1.MoveIntent.verify|verify} messages.
             * @param message MoveIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.MoveIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MoveIntent message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape} MoveIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape;

            /**
             * Decodes a MoveIntent message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape} MoveIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape;

            /**
             * Verifies a MoveIntent message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for MoveIntent
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MoveIntent {

            /** Properties of a MoveIntent. */
            interface $Properties {

                /** MoveIntent scene_epoch */
                scene_epoch?: (Long|null);

                /** MoveIntent input_sequence */
                input_sequence?: (number|null);

                /** MoveIntent destination */
                destination?: (minimir.v1.GridPoint.$Properties|null);

                /** MoveIntent direction */
                direction?: (number|null);

                /** MoveIntent release_joystick */
                release_joystick?: (boolean|null);

                /** MoveIntent cancel_navigation */
                cancel_navigation?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MoveIntent. */
            type $Shape = minimir.v1.MoveIntent.$Properties;
        }

        /**
         * Properties of a CastIntent.
         * @deprecated Use minimir.v1.CastIntent.$Properties instead.
         */
        interface ICastIntent extends minimir.v1.CastIntent.$Properties {
        }

        /** Represents a CastIntent. */
        class CastIntent {

            /**
             * Constructs a new CastIntent.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CastIntent.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CastIntent scene_epoch. */
            scene_epoch: Long;

            /** CastIntent input_sequence. */
            input_sequence: number;

            /** CastIntent skill_id. */
            skill_id: number;

            /** CastIntent target_handle. */
            target_handle: number;

            /** CastIntent target_grid. */
            target_grid?: (minimir.v1.GridPoint.$Properties|null);

            /** CastIntent direction. */
            direction: number;

            /**
             * Creates a new CastIntent instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CastIntent instance
             */
            static create(properties: minimir.v1.CastIntent.$Shape): minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape;
            static create(properties?: minimir.v1.CastIntent.$Properties): minimir.v1.CastIntent;

            /**
             * Encodes the specified CastIntent message. Does not implicitly {@link minimir.v1.CastIntent.verify|verify} messages.
             * @param message CastIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CastIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CastIntent message, length delimited. Does not implicitly {@link minimir.v1.CastIntent.verify|verify} messages.
             * @param message CastIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CastIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CastIntent message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape} CastIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape;

            /**
             * Decodes a CastIntent message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape} CastIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape;

            /**
             * Verifies a CastIntent message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CastIntent
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CastIntent {

            /** Properties of a CastIntent. */
            interface $Properties {

                /** CastIntent scene_epoch */
                scene_epoch?: (Long|null);

                /** CastIntent input_sequence */
                input_sequence?: (number|null);

                /** CastIntent skill_id */
                skill_id?: (number|null);

                /** CastIntent target_handle */
                target_handle?: (number|null);

                /** CastIntent target_grid */
                target_grid?: (minimir.v1.GridPoint.$Properties|null);

                /** CastIntent direction */
                direction?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CastIntent. */
            type $Shape = minimir.v1.CastIntent.$Properties;
        }

        /**
         * Properties of an AttackIntent.
         * @deprecated Use minimir.v1.AttackIntent.$Properties instead.
         */
        interface IAttackIntent extends minimir.v1.AttackIntent.$Properties {
        }

        /** Represents an AttackIntent. */
        class AttackIntent {

            /**
             * Constructs a new AttackIntent.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.AttackIntent.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** AttackIntent scene_epoch. */
            scene_epoch: Long;

            /** AttackIntent input_sequence. */
            input_sequence: number;

            /** AttackIntent direction. */
            direction: number;

            /** AttackIntent skill_id. */
            skill_id: number;

            /**
             * Creates a new AttackIntent instance using the specified properties.
             * @param [properties] Properties to set
             * @returns AttackIntent instance
             */
            static create(properties: minimir.v1.AttackIntent.$Shape): minimir.v1.AttackIntent & minimir.v1.AttackIntent.$Shape;
            static create(properties?: minimir.v1.AttackIntent.$Properties): minimir.v1.AttackIntent;

            /**
             * Encodes the specified AttackIntent message. Does not implicitly {@link minimir.v1.AttackIntent.verify|verify} messages.
             * @param message AttackIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.AttackIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified AttackIntent message, length delimited. Does not implicitly {@link minimir.v1.AttackIntent.verify|verify} messages.
             * @param message AttackIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.AttackIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an AttackIntent message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.AttackIntent & minimir.v1.AttackIntent.$Shape} AttackIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.AttackIntent & minimir.v1.AttackIntent.$Shape;

            /**
             * Decodes an AttackIntent message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.AttackIntent & minimir.v1.AttackIntent.$Shape} AttackIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.AttackIntent & minimir.v1.AttackIntent.$Shape;

            /**
             * Verifies an AttackIntent message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for AttackIntent
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace AttackIntent {

            /** Properties of an AttackIntent. */
            interface $Properties {

                /** AttackIntent scene_epoch */
                scene_epoch?: (Long|null);

                /** AttackIntent input_sequence */
                input_sequence?: (number|null);

                /** AttackIntent direction */
                direction?: (number|null);

                /** AttackIntent skill_id */
                skill_id?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an AttackIntent. */
            type $Shape = minimir.v1.AttackIntent.$Properties;
        }

        /**
         * Properties of a SkillModeIntent.
         * @deprecated Use minimir.v1.SkillModeIntent.$Properties instead.
         */
        interface ISkillModeIntent extends minimir.v1.SkillModeIntent.$Properties {
        }

        /** Represents a SkillModeIntent. */
        class SkillModeIntent {

            /**
             * Constructs a new SkillModeIntent.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillModeIntent.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillModeIntent scene_epoch. */
            scene_epoch: Long;

            /** SkillModeIntent input_sequence. */
            input_sequence: number;

            /** SkillModeIntent skill_id. */
            skill_id: number;

            /** SkillModeIntent enabled. */
            enabled: boolean;

            /**
             * Creates a new SkillModeIntent instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillModeIntent instance
             */
            static create(properties: minimir.v1.SkillModeIntent.$Shape): minimir.v1.SkillModeIntent & minimir.v1.SkillModeIntent.$Shape;
            static create(properties?: minimir.v1.SkillModeIntent.$Properties): minimir.v1.SkillModeIntent;

            /**
             * Encodes the specified SkillModeIntent message. Does not implicitly {@link minimir.v1.SkillModeIntent.verify|verify} messages.
             * @param message SkillModeIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillModeIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillModeIntent message, length delimited. Does not implicitly {@link minimir.v1.SkillModeIntent.verify|verify} messages.
             * @param message SkillModeIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillModeIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillModeIntent message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillModeIntent & minimir.v1.SkillModeIntent.$Shape} SkillModeIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillModeIntent & minimir.v1.SkillModeIntent.$Shape;

            /**
             * Decodes a SkillModeIntent message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillModeIntent & minimir.v1.SkillModeIntent.$Shape} SkillModeIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillModeIntent & minimir.v1.SkillModeIntent.$Shape;

            /**
             * Verifies a SkillModeIntent message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillModeIntent
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillModeIntent {

            /** Properties of a SkillModeIntent. */
            interface $Properties {

                /** SkillModeIntent scene_epoch */
                scene_epoch?: (Long|null);

                /** SkillModeIntent input_sequence */
                input_sequence?: (number|null);

                /** SkillModeIntent skill_id */
                skill_id?: (number|null);

                /** SkillModeIntent enabled */
                enabled?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillModeIntent. */
            type $Shape = minimir.v1.SkillModeIntent.$Properties;
        }

        /**
         * Properties of a SkillModeEntry.
         * @deprecated Use minimir.v1.SkillModeEntry.$Properties instead.
         */
        interface ISkillModeEntry extends minimir.v1.SkillModeEntry.$Properties {
        }

        /** Represents a SkillModeEntry. */
        class SkillModeEntry {

            /**
             * Constructs a new SkillModeEntry.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillModeEntry.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillModeEntry skill_id. */
            skill_id: number;

            /** SkillModeEntry enabled. */
            enabled: boolean;

            /** SkillModeEntry ready. */
            ready: boolean;

            /** SkillModeEntry remaining_ms. */
            remaining_ms: number;

            /** SkillModeEntry cooldown_remaining_ms. */
            cooldown_remaining_ms: number;

            /**
             * Creates a new SkillModeEntry instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillModeEntry instance
             */
            static create(properties: minimir.v1.SkillModeEntry.$Shape): minimir.v1.SkillModeEntry & minimir.v1.SkillModeEntry.$Shape;
            static create(properties?: minimir.v1.SkillModeEntry.$Properties): minimir.v1.SkillModeEntry;

            /**
             * Encodes the specified SkillModeEntry message. Does not implicitly {@link minimir.v1.SkillModeEntry.verify|verify} messages.
             * @param message SkillModeEntry message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillModeEntry.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillModeEntry message, length delimited. Does not implicitly {@link minimir.v1.SkillModeEntry.verify|verify} messages.
             * @param message SkillModeEntry message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillModeEntry.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillModeEntry message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillModeEntry & minimir.v1.SkillModeEntry.$Shape} SkillModeEntry
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillModeEntry & minimir.v1.SkillModeEntry.$Shape;

            /**
             * Decodes a SkillModeEntry message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillModeEntry & minimir.v1.SkillModeEntry.$Shape} SkillModeEntry
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillModeEntry & minimir.v1.SkillModeEntry.$Shape;

            /**
             * Verifies a SkillModeEntry message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillModeEntry
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillModeEntry {

            /** Properties of a SkillModeEntry. */
            interface $Properties {

                /** SkillModeEntry skill_id */
                skill_id?: (number|null);

                /** SkillModeEntry enabled */
                enabled?: (boolean|null);

                /** SkillModeEntry ready */
                ready?: (boolean|null);

                /** SkillModeEntry remaining_ms */
                remaining_ms?: (number|null);

                /** SkillModeEntry cooldown_remaining_ms */
                cooldown_remaining_ms?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillModeEntry. */
            type $Shape = minimir.v1.SkillModeEntry.$Properties;
        }

        /**
         * Properties of a SkillModeState.
         * @deprecated Use minimir.v1.SkillModeState.$Properties instead.
         */
        interface ISkillModeState extends minimir.v1.SkillModeState.$Properties {
        }

        /** Represents a SkillModeState. */
        class SkillModeState {

            /**
             * Constructs a new SkillModeState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillModeState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillModeState scene_epoch. */
            scene_epoch: Long;

            /** SkillModeState revision. */
            revision: Long;

            /** SkillModeState states. */
            states: minimir.v1.SkillModeEntry.$Properties[];

            /** SkillModeState player_life. */
            player_life: Long;

            /** SkillModeState server_time_unix_ms. */
            server_time_unix_ms: Long;

            /**
             * Creates a new SkillModeState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillModeState instance
             */
            static create(properties: minimir.v1.SkillModeState.$Shape): minimir.v1.SkillModeState & minimir.v1.SkillModeState.$Shape;
            static create(properties?: minimir.v1.SkillModeState.$Properties): minimir.v1.SkillModeState;

            /**
             * Encodes the specified SkillModeState message. Does not implicitly {@link minimir.v1.SkillModeState.verify|verify} messages.
             * @param message SkillModeState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillModeState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillModeState message, length delimited. Does not implicitly {@link minimir.v1.SkillModeState.verify|verify} messages.
             * @param message SkillModeState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillModeState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillModeState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillModeState & minimir.v1.SkillModeState.$Shape} SkillModeState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillModeState & minimir.v1.SkillModeState.$Shape;

            /**
             * Decodes a SkillModeState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillModeState & minimir.v1.SkillModeState.$Shape} SkillModeState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillModeState & minimir.v1.SkillModeState.$Shape;

            /**
             * Verifies a SkillModeState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillModeState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillModeState {

            /** Properties of a SkillModeState. */
            interface $Properties {

                /** SkillModeState scene_epoch */
                scene_epoch?: (Long|null);

                /** SkillModeState revision */
                revision?: (Long|null);

                /** SkillModeState states */
                states?: (minimir.v1.SkillModeEntry.$Properties[]|null);

                /** SkillModeState player_life */
                player_life?: (Long|null);

                /** SkillModeState server_time_unix_ms */
                server_time_unix_ms?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillModeState. */
            type $Shape = minimir.v1.SkillModeState.$Properties;
        }

        /**
         * Properties of a CastStep.
         * @deprecated Use minimir.v1.CastStep.$Properties instead.
         */
        interface ICastStep extends minimir.v1.CastStep.$Properties {
        }

        /** Represents a CastStep. */
        class CastStep {

            /**
             * Constructs a new CastStep.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CastStep.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CastStep scene_epoch. */
            scene_epoch: Long;

            /** CastStep cast_id. */
            cast_id: Long;

            /** CastStep skill_id. */
            skill_id: number;

            /** CastStep caster_handle. */
            caster_handle: number;

            /** CastStep caster_life. */
            caster_life: Long;

            /** CastStep grid. */
            grid?: (minimir.v1.GridPoint.$Properties|null);

            /** CastStep direction. */
            direction: number;

            /** CastStep step_index. */
            step_index: number;

            /** CastStep skill_level. */
            skill_level: number;

            /**
             * Creates a new CastStep instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CastStep instance
             */
            static create(properties: minimir.v1.CastStep.$Shape): minimir.v1.CastStep & minimir.v1.CastStep.$Shape;
            static create(properties?: minimir.v1.CastStep.$Properties): minimir.v1.CastStep;

            /**
             * Encodes the specified CastStep message. Does not implicitly {@link minimir.v1.CastStep.verify|verify} messages.
             * @param message CastStep message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CastStep.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CastStep message, length delimited. Does not implicitly {@link minimir.v1.CastStep.verify|verify} messages.
             * @param message CastStep message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CastStep.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CastStep message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CastStep & minimir.v1.CastStep.$Shape} CastStep
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CastStep & minimir.v1.CastStep.$Shape;

            /**
             * Decodes a CastStep message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CastStep & minimir.v1.CastStep.$Shape} CastStep
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CastStep & minimir.v1.CastStep.$Shape;

            /**
             * Verifies a CastStep message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CastStep
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CastStep {

            /** Properties of a CastStep. */
            interface $Properties {

                /** CastStep scene_epoch */
                scene_epoch?: (Long|null);

                /** CastStep cast_id */
                cast_id?: (Long|null);

                /** CastStep skill_id */
                skill_id?: (number|null);

                /** CastStep caster_handle */
                caster_handle?: (number|null);

                /** CastStep caster_life */
                caster_life?: (Long|null);

                /** CastStep grid */
                grid?: (minimir.v1.GridPoint.$Properties|null);

                /** CastStep direction */
                direction?: (number|null);

                /** CastStep step_index */
                step_index?: (number|null);

                /** CastStep skill_level */
                skill_level?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CastStep. */
            type $Shape = minimir.v1.CastStep.$Properties;
        }

        /**
         * Properties of a CastAccepted.
         * @deprecated Use minimir.v1.CastAccepted.$Properties instead.
         */
        interface ICastAccepted extends minimir.v1.CastAccepted.$Properties {
        }

        /** Represents a CastAccepted. */
        class CastAccepted {

            /**
             * Constructs a new CastAccepted.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CastAccepted.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CastAccepted scene_epoch. */
            scene_epoch: Long;

            /** CastAccepted cast_id. */
            cast_id: Long;

            /** CastAccepted skill_id. */
            skill_id: number;

            /** CastAccepted caster_handle. */
            caster_handle: number;

            /** CastAccepted target_handle. */
            target_handle: number;

            /** CastAccepted origin. */
            origin?: (minimir.v1.GridPoint.$Properties|null);

            /** CastAccepted target_grid. */
            target_grid?: (minimir.v1.GridPoint.$Properties|null);

            /** CastAccepted direction. */
            direction: number;

            /** CastAccepted delay_ms. */
            delay_ms: number;

            /** CastAccepted input_sequence. */
            input_sequence: number;

            /** CastAccepted caster_life. */
            caster_life: Long;

            /** CastAccepted target_life. */
            target_life: Long;

            /** CastAccepted skill_level. */
            skill_level: number;

            /**
             * Creates a new CastAccepted instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CastAccepted instance
             */
            static create(properties: minimir.v1.CastAccepted.$Shape): minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape;
            static create(properties?: minimir.v1.CastAccepted.$Properties): minimir.v1.CastAccepted;

            /**
             * Encodes the specified CastAccepted message. Does not implicitly {@link minimir.v1.CastAccepted.verify|verify} messages.
             * @param message CastAccepted message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CastAccepted.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CastAccepted message, length delimited. Does not implicitly {@link minimir.v1.CastAccepted.verify|verify} messages.
             * @param message CastAccepted message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CastAccepted.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CastAccepted message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape} CastAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape;

            /**
             * Decodes a CastAccepted message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape} CastAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape;

            /**
             * Verifies a CastAccepted message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CastAccepted
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CastAccepted {

            /** Properties of a CastAccepted. */
            interface $Properties {

                /** CastAccepted scene_epoch */
                scene_epoch?: (Long|null);

                /** CastAccepted cast_id */
                cast_id?: (Long|null);

                /** CastAccepted skill_id */
                skill_id?: (number|null);

                /** CastAccepted caster_handle */
                caster_handle?: (number|null);

                /** CastAccepted target_handle */
                target_handle?: (number|null);

                /** CastAccepted origin */
                origin?: (minimir.v1.GridPoint.$Properties|null);

                /** CastAccepted target_grid */
                target_grid?: (minimir.v1.GridPoint.$Properties|null);

                /** CastAccepted direction */
                direction?: (number|null);

                /** CastAccepted delay_ms */
                delay_ms?: (number|null);

                /** CastAccepted input_sequence */
                input_sequence?: (number|null);

                /** CastAccepted caster_life */
                caster_life?: (Long|null);

                /** CastAccepted target_life */
                target_life?: (Long|null);

                /** CastAccepted skill_level */
                skill_level?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CastAccepted. */
            type $Shape = minimir.v1.CastAccepted.$Properties;
        }

        /**
         * Properties of an AttackAccepted.
         * @deprecated Use minimir.v1.AttackAccepted.$Properties instead.
         */
        interface IAttackAccepted extends minimir.v1.AttackAccepted.$Properties {
        }

        /** Represents an AttackAccepted. */
        class AttackAccepted {

            /**
             * Constructs a new AttackAccepted.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.AttackAccepted.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** AttackAccepted scene_epoch. */
            scene_epoch: Long;

            /** AttackAccepted event_id. */
            event_id: Long;

            /** AttackAccepted caster_handle. */
            caster_handle: number;

            /** AttackAccepted target_handle. */
            target_handle: number;

            /** AttackAccepted origin. */
            origin?: (minimir.v1.GridPoint.$Properties|null);

            /** AttackAccepted target_grid. */
            target_grid?: (minimir.v1.GridPoint.$Properties|null);

            /** AttackAccepted direction. */
            direction: number;

            /** AttackAccepted delay_ms. */
            delay_ms: number;

            /** AttackAccepted attack_sequence. */
            attack_sequence: number;

            /** AttackAccepted input_sequence. */
            input_sequence: number;

            /** AttackAccepted skill_id. */
            skill_id: number;

            /** AttackAccepted skill_level. */
            skill_level: number;

            /** AttackAccepted caster_life. */
            caster_life: Long;

            /**
             * Creates a new AttackAccepted instance using the specified properties.
             * @param [properties] Properties to set
             * @returns AttackAccepted instance
             */
            static create(properties: minimir.v1.AttackAccepted.$Shape): minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape;
            static create(properties?: minimir.v1.AttackAccepted.$Properties): minimir.v1.AttackAccepted;

            /**
             * Encodes the specified AttackAccepted message. Does not implicitly {@link minimir.v1.AttackAccepted.verify|verify} messages.
             * @param message AttackAccepted message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.AttackAccepted.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified AttackAccepted message, length delimited. Does not implicitly {@link minimir.v1.AttackAccepted.verify|verify} messages.
             * @param message AttackAccepted message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.AttackAccepted.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an AttackAccepted message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape} AttackAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape;

            /**
             * Decodes an AttackAccepted message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape} AttackAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape;

            /**
             * Verifies an AttackAccepted message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for AttackAccepted
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace AttackAccepted {

            /** Properties of an AttackAccepted. */
            interface $Properties {

                /** AttackAccepted scene_epoch */
                scene_epoch?: (Long|null);

                /** AttackAccepted event_id */
                event_id?: (Long|null);

                /** AttackAccepted caster_handle */
                caster_handle?: (number|null);

                /** AttackAccepted target_handle */
                target_handle?: (number|null);

                /** AttackAccepted origin */
                origin?: (minimir.v1.GridPoint.$Properties|null);

                /** AttackAccepted target_grid */
                target_grid?: (minimir.v1.GridPoint.$Properties|null);

                /** AttackAccepted direction */
                direction?: (number|null);

                /** AttackAccepted delay_ms */
                delay_ms?: (number|null);

                /** AttackAccepted attack_sequence */
                attack_sequence?: (number|null);

                /** AttackAccepted input_sequence */
                input_sequence?: (number|null);

                /** AttackAccepted skill_id */
                skill_id?: (number|null);

                /** AttackAccepted skill_level */
                skill_level?: (number|null);

                /** AttackAccepted caster_life */
                caster_life?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an AttackAccepted. */
            type $Shape = minimir.v1.AttackAccepted.$Properties;
        }

        /**
         * Properties of a CombatImpact.
         * @deprecated Use minimir.v1.CombatImpact.$Properties instead.
         */
        interface ICombatImpact extends minimir.v1.CombatImpact.$Properties {
        }

        /** Represents a CombatImpact. */
        class CombatImpact {

            /**
             * Constructs a new CombatImpact.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CombatImpact.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CombatImpact scene_epoch. */
            scene_epoch: Long;

            /** CombatImpact event_id. */
            event_id: Long;

            /** CombatImpact skill_id. */
            skill_id: number;

            /** CombatImpact caster_handle. */
            caster_handle: number;

            /** CombatImpact target_handle. */
            target_handle: number;

            /** CombatImpact target_life. */
            target_life: Long;

            /** CombatImpact target_grid. */
            target_grid?: (minimir.v1.GridPoint.$Properties|null);

            /** CombatImpact amount. */
            amount: Long;

            /** CombatImpact outcome. */
            outcome: number;

            /** CombatImpact healing. */
            healing: boolean;

            /** CombatImpact cast_id. */
            cast_id: Long;

            /** CombatImpact attack_sequence. */
            attack_sequence: number;

            /**
             * Creates a new CombatImpact instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CombatImpact instance
             */
            static create(properties: minimir.v1.CombatImpact.$Shape): minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape;
            static create(properties?: minimir.v1.CombatImpact.$Properties): minimir.v1.CombatImpact;

            /**
             * Encodes the specified CombatImpact message. Does not implicitly {@link minimir.v1.CombatImpact.verify|verify} messages.
             * @param message CombatImpact message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CombatImpact.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CombatImpact message, length delimited. Does not implicitly {@link minimir.v1.CombatImpact.verify|verify} messages.
             * @param message CombatImpact message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CombatImpact.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CombatImpact message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape} CombatImpact
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape;

            /**
             * Decodes a CombatImpact message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape} CombatImpact
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape;

            /**
             * Verifies a CombatImpact message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CombatImpact
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CombatImpact {

            /** Properties of a CombatImpact. */
            interface $Properties {

                /** CombatImpact scene_epoch */
                scene_epoch?: (Long|null);

                /** CombatImpact event_id */
                event_id?: (Long|null);

                /** CombatImpact skill_id */
                skill_id?: (number|null);

                /** CombatImpact caster_handle */
                caster_handle?: (number|null);

                /** CombatImpact target_handle */
                target_handle?: (number|null);

                /** CombatImpact target_life */
                target_life?: (Long|null);

                /** CombatImpact target_grid */
                target_grid?: (minimir.v1.GridPoint.$Properties|null);

                /** CombatImpact amount */
                amount?: (Long|null);

                /** CombatImpact outcome */
                outcome?: (number|null);

                /** CombatImpact healing */
                healing?: (boolean|null);

                /** CombatImpact cast_id */
                cast_id?: (Long|null);

                /** CombatImpact attack_sequence */
                attack_sequence?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CombatImpact. */
            type $Shape = minimir.v1.CombatImpact.$Properties;
        }

        /**
         * Properties of an AutoBattleIntent.
         * @deprecated Use minimir.v1.AutoBattleIntent.$Properties instead.
         */
        interface IAutoBattleIntent extends minimir.v1.AutoBattleIntent.$Properties {
        }

        /** Represents an AutoBattleIntent. */
        class AutoBattleIntent {

            /**
             * Constructs a new AutoBattleIntent.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.AutoBattleIntent.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** AutoBattleIntent scene_epoch. */
            scene_epoch: Long;

            /** AutoBattleIntent enabled. */
            enabled: boolean;

            /** AutoBattleIntent monster_count. */
            monster_count: number;

            /**
             * Creates a new AutoBattleIntent instance using the specified properties.
             * @param [properties] Properties to set
             * @returns AutoBattleIntent instance
             */
            static create(properties: minimir.v1.AutoBattleIntent.$Shape): minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape;
            static create(properties?: minimir.v1.AutoBattleIntent.$Properties): minimir.v1.AutoBattleIntent;

            /**
             * Encodes the specified AutoBattleIntent message. Does not implicitly {@link minimir.v1.AutoBattleIntent.verify|verify} messages.
             * @param message AutoBattleIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.AutoBattleIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified AutoBattleIntent message, length delimited. Does not implicitly {@link minimir.v1.AutoBattleIntent.verify|verify} messages.
             * @param message AutoBattleIntent message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.AutoBattleIntent.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an AutoBattleIntent message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape} AutoBattleIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape;

            /**
             * Decodes an AutoBattleIntent message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape} AutoBattleIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape;

            /**
             * Verifies an AutoBattleIntent message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for AutoBattleIntent
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace AutoBattleIntent {

            /** Properties of an AutoBattleIntent. */
            interface $Properties {

                /** AutoBattleIntent scene_epoch */
                scene_epoch?: (Long|null);

                /** AutoBattleIntent enabled */
                enabled?: (boolean|null);

                /** AutoBattleIntent monster_count */
                monster_count?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an AutoBattleIntent. */
            type $Shape = minimir.v1.AutoBattleIntent.$Properties;
        }

        /**
         * Properties of a HostingState.
         * @deprecated Use minimir.v1.HostingState.$Properties instead.
         */
        interface IHostingState extends minimir.v1.HostingState.$Properties {
        }

        /** Represents a HostingState. */
        class HostingState {

            /**
             * Constructs a new HostingState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.HostingState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** HostingState remain_hosting_sec. */
            remain_hosting_sec: number;

            /** HostingState auto_battle_enabled. */
            auto_battle_enabled: boolean;

            /** HostingState state_revision. */
            state_revision: Long;

            /**
             * Creates a new HostingState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns HostingState instance
             */
            static create(properties: minimir.v1.HostingState.$Shape): minimir.v1.HostingState & minimir.v1.HostingState.$Shape;
            static create(properties?: minimir.v1.HostingState.$Properties): minimir.v1.HostingState;

            /**
             * Encodes the specified HostingState message. Does not implicitly {@link minimir.v1.HostingState.verify|verify} messages.
             * @param message HostingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.HostingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified HostingState message, length delimited. Does not implicitly {@link minimir.v1.HostingState.verify|verify} messages.
             * @param message HostingState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.HostingState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a HostingState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.HostingState & minimir.v1.HostingState.$Shape} HostingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.HostingState & minimir.v1.HostingState.$Shape;

            /**
             * Decodes a HostingState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.HostingState & minimir.v1.HostingState.$Shape} HostingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.HostingState & minimir.v1.HostingState.$Shape;

            /**
             * Verifies a HostingState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for HostingState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace HostingState {

            /** Properties of a HostingState. */
            interface $Properties {

                /** HostingState remain_hosting_sec */
                remain_hosting_sec?: (number|null);

                /** HostingState auto_battle_enabled */
                auto_battle_enabled?: (boolean|null);

                /** HostingState state_revision */
                state_revision?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a HostingState. */
            type $Shape = minimir.v1.HostingState.$Properties;
        }

        /**
         * Properties of a DeathNotice.
         * @deprecated Use minimir.v1.DeathNotice.$Properties instead.
         */
        interface IDeathNotice extends minimir.v1.DeathNotice.$Properties {
        }

        /** Represents a DeathNotice. */
        class DeathNotice {

            /**
             * Constructs a new DeathNotice.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.DeathNotice.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** DeathNotice death_id. */
            death_id: Uint8Array;

            /** DeathNotice cause_code. */
            cause_code: number;

            /** DeathNotice allowed_actions. */
            allowed_actions: number;

            /** DeathNotice inplace_price_yuanbao_units. */
            inplace_price_yuanbao_units: Long;

            /** DeathNotice rule_version. */
            rule_version: Long;

            /**
             * Creates a new DeathNotice instance using the specified properties.
             * @param [properties] Properties to set
             * @returns DeathNotice instance
             */
            static create(properties: minimir.v1.DeathNotice.$Shape): minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape;
            static create(properties?: minimir.v1.DeathNotice.$Properties): minimir.v1.DeathNotice;

            /**
             * Encodes the specified DeathNotice message. Does not implicitly {@link minimir.v1.DeathNotice.verify|verify} messages.
             * @param message DeathNotice message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.DeathNotice.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified DeathNotice message, length delimited. Does not implicitly {@link minimir.v1.DeathNotice.verify|verify} messages.
             * @param message DeathNotice message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.DeathNotice.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a DeathNotice message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape} DeathNotice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape;

            /**
             * Decodes a DeathNotice message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape} DeathNotice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape;

            /**
             * Verifies a DeathNotice message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for DeathNotice
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace DeathNotice {

            /** Properties of a DeathNotice. */
            interface $Properties {

                /** DeathNotice death_id */
                death_id?: (Uint8Array|null);

                /** DeathNotice cause_code */
                cause_code?: (number|null);

                /** DeathNotice allowed_actions */
                allowed_actions?: (number|null);

                /** DeathNotice inplace_price_yuanbao_units */
                inplace_price_yuanbao_units?: (Long|null);

                /** DeathNotice rule_version */
                rule_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a DeathNotice. */
            type $Shape = minimir.v1.DeathNotice.$Properties;
        }

        /**
         * Properties of a ReviveChoice.
         * @deprecated Use minimir.v1.ReviveChoice.$Properties instead.
         */
        interface IReviveChoice extends minimir.v1.ReviveChoice.$Properties {
        }

        /** Represents a ReviveChoice. */
        class ReviveChoice {

            /**
             * Constructs a new ReviveChoice.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ReviveChoice.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ReviveChoice death_id. */
            death_id: Uint8Array;

            /** ReviveChoice action. */
            action: number;

            /** ReviveChoice operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new ReviveChoice instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ReviveChoice instance
             */
            static create(properties: minimir.v1.ReviveChoice.$Shape): minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape;
            static create(properties?: minimir.v1.ReviveChoice.$Properties): minimir.v1.ReviveChoice;

            /**
             * Encodes the specified ReviveChoice message. Does not implicitly {@link minimir.v1.ReviveChoice.verify|verify} messages.
             * @param message ReviveChoice message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ReviveChoice.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ReviveChoice message, length delimited. Does not implicitly {@link minimir.v1.ReviveChoice.verify|verify} messages.
             * @param message ReviveChoice message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ReviveChoice.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ReviveChoice message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape} ReviveChoice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape;

            /**
             * Decodes a ReviveChoice message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape} ReviveChoice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape;

            /**
             * Verifies a ReviveChoice message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ReviveChoice
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ReviveChoice {

            /** Properties of a ReviveChoice. */
            interface $Properties {

                /** ReviveChoice death_id */
                death_id?: (Uint8Array|null);

                /** ReviveChoice action */
                action?: (number|null);

                /** ReviveChoice operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ReviveChoice. */
            type $Shape = minimir.v1.ReviveChoice.$Properties;
        }

        /**
         * Properties of a WalletState.
         * @deprecated Use minimir.v1.WalletState.$Properties instead.
         */
        interface IWalletState extends minimir.v1.WalletState.$Properties {
        }

        /** Represents a WalletState. */
        class WalletState {

            /**
             * Constructs a new WalletState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WalletState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WalletState gold. */
            gold: Long;

            /** WalletState bound_gold. */
            bound_gold: Long;

            /** WalletState yuanbao_units. */
            yuanbao_units: Long;

            /** WalletState bound_yuanbao_units. */
            bound_yuanbao_units: Long;

            /** WalletState revision. */
            revision: Long;

            /**
             * Creates a new WalletState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WalletState instance
             */
            static create(properties: minimir.v1.WalletState.$Shape): minimir.v1.WalletState & minimir.v1.WalletState.$Shape;
            static create(properties?: minimir.v1.WalletState.$Properties): minimir.v1.WalletState;

            /**
             * Encodes the specified WalletState message. Does not implicitly {@link minimir.v1.WalletState.verify|verify} messages.
             * @param message WalletState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WalletState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WalletState message, length delimited. Does not implicitly {@link minimir.v1.WalletState.verify|verify} messages.
             * @param message WalletState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WalletState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WalletState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WalletState & minimir.v1.WalletState.$Shape} WalletState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WalletState & minimir.v1.WalletState.$Shape;

            /**
             * Decodes a WalletState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WalletState & minimir.v1.WalletState.$Shape} WalletState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WalletState & minimir.v1.WalletState.$Shape;

            /**
             * Verifies a WalletState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WalletState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WalletState {

            /** Properties of a WalletState. */
            interface $Properties {

                /** WalletState gold */
                gold?: (Long|null);

                /** WalletState bound_gold */
                bound_gold?: (Long|null);

                /** WalletState yuanbao_units */
                yuanbao_units?: (Long|null);

                /** WalletState bound_yuanbao_units */
                bound_yuanbao_units?: (Long|null);

                /** WalletState revision */
                revision?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WalletState. */
            type $Shape = minimir.v1.WalletState.$Properties;
        }

        /**
         * Properties of an ItemView.
         * @deprecated Use minimir.v1.ItemView.$Properties instead.
         */
        interface IItemView extends minimir.v1.ItemView.$Properties {
        }

        /** Represents an ItemView. */
        class ItemView {

            /**
             * Constructs a new ItemView.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ItemView.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ItemView instance_id. */
            instance_id: Uint8Array;

            /** ItemView item_id. */
            item_id: number;

            /** ItemView quantity. */
            quantity: Long;

            /** ItemView bound. */
            bound: boolean;

            /** ItemView version. */
            version: Long;

            /** ItemView source_kind. */
            source_kind: string;

            /** ItemView source_id. */
            source_id: string;

            /** ItemView quality. */
            quality: number;

            /** ItemView locked. */
            locked: boolean;

            /** ItemView recycle_block_flags. */
            recycle_block_flags: number;

            /**
             * Creates a new ItemView instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ItemView instance
             */
            static create(properties: minimir.v1.ItemView.$Shape): minimir.v1.ItemView & minimir.v1.ItemView.$Shape;
            static create(properties?: minimir.v1.ItemView.$Properties): minimir.v1.ItemView;

            /**
             * Encodes the specified ItemView message. Does not implicitly {@link minimir.v1.ItemView.verify|verify} messages.
             * @param message ItemView message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ItemView.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ItemView message, length delimited. Does not implicitly {@link minimir.v1.ItemView.verify|verify} messages.
             * @param message ItemView message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ItemView.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an ItemView message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ItemView & minimir.v1.ItemView.$Shape} ItemView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ItemView & minimir.v1.ItemView.$Shape;

            /**
             * Decodes an ItemView message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ItemView & minimir.v1.ItemView.$Shape} ItemView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ItemView & minimir.v1.ItemView.$Shape;

            /**
             * Verifies an ItemView message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ItemView
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ItemView {

            /** Properties of an ItemView. */
            interface $Properties {

                /** ItemView instance_id */
                instance_id?: (Uint8Array|null);

                /** ItemView item_id */
                item_id?: (number|null);

                /** ItemView quantity */
                quantity?: (Long|null);

                /** ItemView bound */
                bound?: (boolean|null);

                /** ItemView version */
                version?: (Long|null);

                /** ItemView source_kind */
                source_kind?: (string|null);

                /** ItemView source_id */
                source_id?: (string|null);

                /** ItemView quality */
                quality?: (number|null);

                /** ItemView locked */
                locked?: (boolean|null);

                /** ItemView recycle_block_flags */
                recycle_block_flags?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an ItemView. */
            type $Shape = minimir.v1.ItemView.$Properties;
        }

        /**
         * Properties of an EquippedItem.
         * @deprecated Use minimir.v1.EquippedItem.$Properties instead.
         */
        interface IEquippedItem extends minimir.v1.EquippedItem.$Properties {
        }

        /** Represents an EquippedItem. */
        class EquippedItem {

            /**
             * Constructs a new EquippedItem.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EquippedItem.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EquippedItem slot. */
            slot: number;

            /** EquippedItem item. */
            item?: (minimir.v1.ItemView.$Properties|null);

            /**
             * Creates a new EquippedItem instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EquippedItem instance
             */
            static create(properties: minimir.v1.EquippedItem.$Shape): minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape;
            static create(properties?: minimir.v1.EquippedItem.$Properties): minimir.v1.EquippedItem;

            /**
             * Encodes the specified EquippedItem message. Does not implicitly {@link minimir.v1.EquippedItem.verify|verify} messages.
             * @param message EquippedItem message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EquippedItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EquippedItem message, length delimited. Does not implicitly {@link minimir.v1.EquippedItem.verify|verify} messages.
             * @param message EquippedItem message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EquippedItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EquippedItem message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape} EquippedItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape;

            /**
             * Decodes an EquippedItem message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape} EquippedItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape;

            /**
             * Verifies an EquippedItem message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EquippedItem
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EquippedItem {

            /** Properties of an EquippedItem. */
            interface $Properties {

                /** EquippedItem slot */
                slot?: (number|null);

                /** EquippedItem item */
                item?: (minimir.v1.ItemView.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EquippedItem. */
            type $Shape = minimir.v1.EquippedItem.$Properties;
        }

        /**
         * Properties of a LearnedSkill.
         * @deprecated Use minimir.v1.LearnedSkill.$Properties instead.
         */
        interface ILearnedSkill extends minimir.v1.LearnedSkill.$Properties {
        }

        /** Represents a LearnedSkill. */
        class LearnedSkill {

            /**
             * Constructs a new LearnedSkill.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.LearnedSkill.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** LearnedSkill skill_id. */
            skill_id: number;

            /** LearnedSkill level. */
            level: number;

            /**
             * Creates a new LearnedSkill instance using the specified properties.
             * @param [properties] Properties to set
             * @returns LearnedSkill instance
             */
            static create(properties: minimir.v1.LearnedSkill.$Shape): minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape;
            static create(properties?: minimir.v1.LearnedSkill.$Properties): minimir.v1.LearnedSkill;

            /**
             * Encodes the specified LearnedSkill message. Does not implicitly {@link minimir.v1.LearnedSkill.verify|verify} messages.
             * @param message LearnedSkill message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.LearnedSkill.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified LearnedSkill message, length delimited. Does not implicitly {@link minimir.v1.LearnedSkill.verify|verify} messages.
             * @param message LearnedSkill message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.LearnedSkill.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a LearnedSkill message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape} LearnedSkill
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape;

            /**
             * Decodes a LearnedSkill message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape} LearnedSkill
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape;

            /**
             * Verifies a LearnedSkill message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for LearnedSkill
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace LearnedSkill {

            /** Properties of a LearnedSkill. */
            interface $Properties {

                /** LearnedSkill skill_id */
                skill_id?: (number|null);

                /** LearnedSkill level */
                level?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a LearnedSkill. */
            type $Shape = minimir.v1.LearnedSkill.$Properties;
        }

        /**
         * Properties of an InventoryResult.
         * @deprecated Use minimir.v1.InventoryResult.$Properties instead.
         */
        interface IInventoryResult extends minimir.v1.InventoryResult.$Properties {
        }

        /** Represents an InventoryResult. */
        class InventoryResult {

            /**
             * Constructs a new InventoryResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.InventoryResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** InventoryResult items. */
            items: minimir.v1.ItemView.$Properties[];

            /** InventoryResult capacity. */
            capacity: number;

            /** InventoryResult revision. */
            revision: Long;

            /** InventoryResult equipped. */
            equipped: minimir.v1.EquippedItem.$Properties[];

            /** InventoryResult level. */
            level: number;

            /** InventoryResult experience. */
            experience: Long;

            /** InventoryResult learned_skills. */
            learned_skills: minimir.v1.LearnedSkill.$Properties[];

            /** InventoryResult auto_recycle. */
            auto_recycle?: (minimir.v1.AutoRecyclePolicy.$Properties|null);

            /** InventoryResult progression. */
            progression?: (minimir.v1.ProgressionState.$Properties|null);

            /**
             * Creates a new InventoryResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns InventoryResult instance
             */
            static create(properties: minimir.v1.InventoryResult.$Shape): minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape;
            static create(properties?: minimir.v1.InventoryResult.$Properties): minimir.v1.InventoryResult;

            /**
             * Encodes the specified InventoryResult message. Does not implicitly {@link minimir.v1.InventoryResult.verify|verify} messages.
             * @param message InventoryResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.InventoryResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified InventoryResult message, length delimited. Does not implicitly {@link minimir.v1.InventoryResult.verify|verify} messages.
             * @param message InventoryResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.InventoryResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an InventoryResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape} InventoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape;

            /**
             * Decodes an InventoryResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape} InventoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape;

            /**
             * Verifies an InventoryResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for InventoryResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace InventoryResult {

            /** Properties of an InventoryResult. */
            interface $Properties {

                /** InventoryResult items */
                items?: (minimir.v1.ItemView.$Properties[]|null);

                /** InventoryResult capacity */
                capacity?: (number|null);

                /** InventoryResult revision */
                revision?: (Long|null);

                /** InventoryResult equipped */
                equipped?: (minimir.v1.EquippedItem.$Properties[]|null);

                /** InventoryResult level */
                level?: (number|null);

                /** InventoryResult experience */
                experience?: (Long|null);

                /** InventoryResult learned_skills */
                learned_skills?: (minimir.v1.LearnedSkill.$Properties[]|null);

                /** InventoryResult auto_recycle */
                auto_recycle?: (minimir.v1.AutoRecyclePolicy.$Properties|null);

                /** InventoryResult progression */
                progression?: (minimir.v1.ProgressionState.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an InventoryResult. */
            type $Shape = minimir.v1.InventoryResult.$Properties;
        }

        /**
         * Properties of a ProgressionState.
         * @deprecated Use minimir.v1.ProgressionState.$Properties instead.
         */
        interface IProgressionState extends minimir.v1.ProgressionState.$Properties {
        }

        /** Represents a ProgressionState. */
        class ProgressionState {

            /**
             * Constructs a new ProgressionState.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ProgressionState.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ProgressionState format_version. */
            format_version: number;

            /** ProgressionState curve_version. */
            curve_version: string;

            /** ProgressionState level_cap. */
            level_cap: number;

            /** ProgressionState rebirth_count. */
            rebirth_count: number;

            /** ProgressionState next_level_experience. */
            next_level_experience: Long;

            /** ProgressionState pending_experience. */
            pending_experience: Long;

            /** ProgressionState pending_experience_limit. */
            pending_experience_limit: Long;

            /** ProgressionState legacy_experience_overflow. */
            legacy_experience_overflow: Long;

            /** ProgressionState at_cap. */
            at_cap: boolean;

            /** ProgressionState migration_required. */
            migration_required: boolean;

            /** ProgressionState rebirth_enabled. */
            rebirth_enabled: boolean;

            /** ProgressionState catalog_max_level. */
            catalog_max_level: number;

            /**
             * Creates a new ProgressionState instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ProgressionState instance
             */
            static create(properties: minimir.v1.ProgressionState.$Shape): minimir.v1.ProgressionState & minimir.v1.ProgressionState.$Shape;
            static create(properties?: minimir.v1.ProgressionState.$Properties): minimir.v1.ProgressionState;

            /**
             * Encodes the specified ProgressionState message. Does not implicitly {@link minimir.v1.ProgressionState.verify|verify} messages.
             * @param message ProgressionState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ProgressionState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ProgressionState message, length delimited. Does not implicitly {@link minimir.v1.ProgressionState.verify|verify} messages.
             * @param message ProgressionState message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ProgressionState.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ProgressionState message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ProgressionState & minimir.v1.ProgressionState.$Shape} ProgressionState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ProgressionState & minimir.v1.ProgressionState.$Shape;

            /**
             * Decodes a ProgressionState message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ProgressionState & minimir.v1.ProgressionState.$Shape} ProgressionState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ProgressionState & minimir.v1.ProgressionState.$Shape;

            /**
             * Verifies a ProgressionState message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ProgressionState
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ProgressionState {

            /** Properties of a ProgressionState. */
            interface $Properties {

                /** ProgressionState format_version */
                format_version?: (number|null);

                /** ProgressionState curve_version */
                curve_version?: (string|null);

                /** ProgressionState level_cap */
                level_cap?: (number|null);

                /** ProgressionState rebirth_count */
                rebirth_count?: (number|null);

                /** ProgressionState next_level_experience */
                next_level_experience?: (Long|null);

                /** ProgressionState pending_experience */
                pending_experience?: (Long|null);

                /** ProgressionState pending_experience_limit */
                pending_experience_limit?: (Long|null);

                /** ProgressionState legacy_experience_overflow */
                legacy_experience_overflow?: (Long|null);

                /** ProgressionState at_cap */
                at_cap?: (boolean|null);

                /** ProgressionState migration_required */
                migration_required?: (boolean|null);

                /** ProgressionState rebirth_enabled */
                rebirth_enabled?: (boolean|null);

                /** ProgressionState catalog_max_level */
                catalog_max_level?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ProgressionState. */
            type $Shape = minimir.v1.ProgressionState.$Properties;
        }

        /**
         * Properties of an ExperienceGrantResult.
         * @deprecated Use minimir.v1.ExperienceGrantResult.$Properties instead.
         */
        interface IExperienceGrantResult extends minimir.v1.ExperienceGrantResult.$Properties {
        }

        /** Represents an ExperienceGrantResult. */
        class ExperienceGrantResult {

            /**
             * Constructs a new ExperienceGrantResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ExperienceGrantResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ExperienceGrantResult role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** ExperienceGrantResult role_revision. */
            role_revision: Long;

            /** ExperienceGrantResult source_seq. */
            source_seq: Long;

            /** ExperienceGrantResult curve_version. */
            curve_version: string;

            /** ExperienceGrantResult raw_experience. */
            raw_experience: Long;

            /** ExperienceGrantResult accepted_experience. */
            accepted_experience: Long;

            /** ExperienceGrantResult pooled_experience. */
            pooled_experience: Long;

            /** ExperienceGrantResult rejected_experience. */
            rejected_experience: Long;

            /** ExperienceGrantResult level_before. */
            level_before: number;

            /** ExperienceGrantResult level_after. */
            level_after: number;

            /** ExperienceGrantResult state. */
            state?: (minimir.v1.ProgressionState.$Properties|null);

            /** ExperienceGrantResult migrated. */
            migrated: boolean;

            /** ExperienceGrantResult legacy_level_before. */
            legacy_level_before: number;

            /** ExperienceGrantResult legacy_level_after. */
            legacy_level_after: number;

            /** ExperienceGrantResult rule_version. */
            rule_version: Long;

            /**
             * Creates a new ExperienceGrantResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ExperienceGrantResult instance
             */
            static create(properties: minimir.v1.ExperienceGrantResult.$Shape): minimir.v1.ExperienceGrantResult & minimir.v1.ExperienceGrantResult.$Shape;
            static create(properties?: minimir.v1.ExperienceGrantResult.$Properties): minimir.v1.ExperienceGrantResult;

            /**
             * Encodes the specified ExperienceGrantResult message. Does not implicitly {@link minimir.v1.ExperienceGrantResult.verify|verify} messages.
             * @param message ExperienceGrantResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ExperienceGrantResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ExperienceGrantResult message, length delimited. Does not implicitly {@link minimir.v1.ExperienceGrantResult.verify|verify} messages.
             * @param message ExperienceGrantResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ExperienceGrantResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an ExperienceGrantResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ExperienceGrantResult & minimir.v1.ExperienceGrantResult.$Shape} ExperienceGrantResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ExperienceGrantResult & minimir.v1.ExperienceGrantResult.$Shape;

            /**
             * Decodes an ExperienceGrantResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ExperienceGrantResult & minimir.v1.ExperienceGrantResult.$Shape} ExperienceGrantResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ExperienceGrantResult & minimir.v1.ExperienceGrantResult.$Shape;

            /**
             * Verifies an ExperienceGrantResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ExperienceGrantResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ExperienceGrantResult {

            /** Properties of an ExperienceGrantResult. */
            interface $Properties {

                /** ExperienceGrantResult role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** ExperienceGrantResult role_revision */
                role_revision?: (Long|null);

                /** ExperienceGrantResult source_seq */
                source_seq?: (Long|null);

                /** ExperienceGrantResult curve_version */
                curve_version?: (string|null);

                /** ExperienceGrantResult raw_experience */
                raw_experience?: (Long|null);

                /** ExperienceGrantResult accepted_experience */
                accepted_experience?: (Long|null);

                /** ExperienceGrantResult pooled_experience */
                pooled_experience?: (Long|null);

                /** ExperienceGrantResult rejected_experience */
                rejected_experience?: (Long|null);

                /** ExperienceGrantResult level_before */
                level_before?: (number|null);

                /** ExperienceGrantResult level_after */
                level_after?: (number|null);

                /** ExperienceGrantResult state */
                state?: (minimir.v1.ProgressionState.$Properties|null);

                /** ExperienceGrantResult migrated */
                migrated?: (boolean|null);

                /** ExperienceGrantResult legacy_level_before */
                legacy_level_before?: (number|null);

                /** ExperienceGrantResult legacy_level_after */
                legacy_level_after?: (number|null);

                /** ExperienceGrantResult rule_version */
                rule_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an ExperienceGrantResult. */
            type $Shape = minimir.v1.ExperienceGrantResult.$Properties;
        }

        /**
         * Properties of an AutoRecyclePolicy.
         * @deprecated Use minimir.v1.AutoRecyclePolicy.$Properties instead.
         */
        interface IAutoRecyclePolicy extends minimir.v1.AutoRecyclePolicy.$Properties {
        }

        /** Represents an AutoRecyclePolicy. */
        class AutoRecyclePolicy {

            /**
             * Constructs a new AutoRecyclePolicy.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.AutoRecyclePolicy.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** AutoRecyclePolicy enabled. */
            enabled: boolean;

            /** AutoRecyclePolicy quality_ids. */
            quality_ids: number[];

            /** AutoRecyclePolicy max_required_level. */
            max_required_level: number;

            /** AutoRecyclePolicy protected_item_ids. */
            protected_item_ids: number[];

            /** AutoRecyclePolicy protect_bound. */
            protect_bound: boolean;

            /** AutoRecyclePolicy protect_unbound. */
            protect_unbound: boolean;

            /** AutoRecyclePolicy revision. */
            revision: Long;

            /**
             * Creates a new AutoRecyclePolicy instance using the specified properties.
             * @param [properties] Properties to set
             * @returns AutoRecyclePolicy instance
             */
            static create(properties: minimir.v1.AutoRecyclePolicy.$Shape): minimir.v1.AutoRecyclePolicy & minimir.v1.AutoRecyclePolicy.$Shape;
            static create(properties?: minimir.v1.AutoRecyclePolicy.$Properties): minimir.v1.AutoRecyclePolicy;

            /**
             * Encodes the specified AutoRecyclePolicy message. Does not implicitly {@link minimir.v1.AutoRecyclePolicy.verify|verify} messages.
             * @param message AutoRecyclePolicy message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.AutoRecyclePolicy.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified AutoRecyclePolicy message, length delimited. Does not implicitly {@link minimir.v1.AutoRecyclePolicy.verify|verify} messages.
             * @param message AutoRecyclePolicy message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.AutoRecyclePolicy.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an AutoRecyclePolicy message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.AutoRecyclePolicy & minimir.v1.AutoRecyclePolicy.$Shape} AutoRecyclePolicy
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.AutoRecyclePolicy & minimir.v1.AutoRecyclePolicy.$Shape;

            /**
             * Decodes an AutoRecyclePolicy message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.AutoRecyclePolicy & minimir.v1.AutoRecyclePolicy.$Shape} AutoRecyclePolicy
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.AutoRecyclePolicy & minimir.v1.AutoRecyclePolicy.$Shape;

            /**
             * Verifies an AutoRecyclePolicy message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for AutoRecyclePolicy
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace AutoRecyclePolicy {

            /** Properties of an AutoRecyclePolicy. */
            interface $Properties {

                /** AutoRecyclePolicy enabled */
                enabled?: (boolean|null);

                /** AutoRecyclePolicy quality_ids */
                quality_ids?: (number[]|null);

                /** AutoRecyclePolicy max_required_level */
                max_required_level?: (number|null);

                /** AutoRecyclePolicy protected_item_ids */
                protected_item_ids?: (number[]|null);

                /** AutoRecyclePolicy protect_bound */
                protect_bound?: (boolean|null);

                /** AutoRecyclePolicy protect_unbound */
                protect_unbound?: (boolean|null);

                /** AutoRecyclePolicy revision */
                revision?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an AutoRecyclePolicy. */
            type $Shape = minimir.v1.AutoRecyclePolicy.$Properties;
        }

        /**
         * Properties of a SetAutoRecycleRequest.
         * @deprecated Use minimir.v1.SetAutoRecycleRequest.$Properties instead.
         */
        interface ISetAutoRecycleRequest extends minimir.v1.SetAutoRecycleRequest.$Properties {
        }

        /** Represents a SetAutoRecycleRequest. */
        class SetAutoRecycleRequest {

            /**
             * Constructs a new SetAutoRecycleRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SetAutoRecycleRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SetAutoRecycleRequest policy. */
            policy?: (minimir.v1.AutoRecyclePolicy.$Properties|null);

            /** SetAutoRecycleRequest apply_existing. */
            apply_existing: boolean;

            /** SetAutoRecycleRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new SetAutoRecycleRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SetAutoRecycleRequest instance
             */
            static create(properties: minimir.v1.SetAutoRecycleRequest.$Shape): minimir.v1.SetAutoRecycleRequest & minimir.v1.SetAutoRecycleRequest.$Shape;
            static create(properties?: minimir.v1.SetAutoRecycleRequest.$Properties): minimir.v1.SetAutoRecycleRequest;

            /**
             * Encodes the specified SetAutoRecycleRequest message. Does not implicitly {@link minimir.v1.SetAutoRecycleRequest.verify|verify} messages.
             * @param message SetAutoRecycleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SetAutoRecycleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SetAutoRecycleRequest message, length delimited. Does not implicitly {@link minimir.v1.SetAutoRecycleRequest.verify|verify} messages.
             * @param message SetAutoRecycleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SetAutoRecycleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SetAutoRecycleRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SetAutoRecycleRequest & minimir.v1.SetAutoRecycleRequest.$Shape} SetAutoRecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SetAutoRecycleRequest & minimir.v1.SetAutoRecycleRequest.$Shape;

            /**
             * Decodes a SetAutoRecycleRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SetAutoRecycleRequest & minimir.v1.SetAutoRecycleRequest.$Shape} SetAutoRecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SetAutoRecycleRequest & minimir.v1.SetAutoRecycleRequest.$Shape;

            /**
             * Verifies a SetAutoRecycleRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SetAutoRecycleRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SetAutoRecycleRequest {

            /** Properties of a SetAutoRecycleRequest. */
            interface $Properties {

                /** SetAutoRecycleRequest policy */
                policy?: (minimir.v1.AutoRecyclePolicy.$Properties|null);

                /** SetAutoRecycleRequest apply_existing */
                apply_existing?: (boolean|null);

                /** SetAutoRecycleRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SetAutoRecycleRequest. */
            type $Shape = minimir.v1.SetAutoRecycleRequest.$Properties;
        }

        /**
         * Properties of a SetItemLockRequest.
         * @deprecated Use minimir.v1.SetItemLockRequest.$Properties instead.
         */
        interface ISetItemLockRequest extends minimir.v1.SetItemLockRequest.$Properties {
        }

        /** Represents a SetItemLockRequest. */
        class SetItemLockRequest {

            /**
             * Constructs a new SetItemLockRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SetItemLockRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SetItemLockRequest item_instance_id. */
            item_instance_id: Uint8Array;

            /** SetItemLockRequest item_version. */
            item_version: Long;

            /** SetItemLockRequest locked. */
            locked: boolean;

            /** SetItemLockRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new SetItemLockRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SetItemLockRequest instance
             */
            static create(properties: minimir.v1.SetItemLockRequest.$Shape): minimir.v1.SetItemLockRequest & minimir.v1.SetItemLockRequest.$Shape;
            static create(properties?: minimir.v1.SetItemLockRequest.$Properties): minimir.v1.SetItemLockRequest;

            /**
             * Encodes the specified SetItemLockRequest message. Does not implicitly {@link minimir.v1.SetItemLockRequest.verify|verify} messages.
             * @param message SetItemLockRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SetItemLockRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SetItemLockRequest message, length delimited. Does not implicitly {@link minimir.v1.SetItemLockRequest.verify|verify} messages.
             * @param message SetItemLockRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SetItemLockRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SetItemLockRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SetItemLockRequest & minimir.v1.SetItemLockRequest.$Shape} SetItemLockRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SetItemLockRequest & minimir.v1.SetItemLockRequest.$Shape;

            /**
             * Decodes a SetItemLockRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SetItemLockRequest & minimir.v1.SetItemLockRequest.$Shape} SetItemLockRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SetItemLockRequest & minimir.v1.SetItemLockRequest.$Shape;

            /**
             * Verifies a SetItemLockRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SetItemLockRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SetItemLockRequest {

            /** Properties of a SetItemLockRequest. */
            interface $Properties {

                /** SetItemLockRequest item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** SetItemLockRequest item_version */
                item_version?: (Long|null);

                /** SetItemLockRequest locked */
                locked?: (boolean|null);

                /** SetItemLockRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SetItemLockRequest. */
            type $Shape = minimir.v1.SetItemLockRequest.$Properties;
        }

        /**
         * Properties of an EquipmentRecycleResult.
         * @deprecated Use minimir.v1.EquipmentRecycleResult.$Properties instead.
         */
        interface IEquipmentRecycleResult extends minimir.v1.EquipmentRecycleResult.$Properties {
        }

        /** Represents an EquipmentRecycleResult. */
        class EquipmentRecycleResult {

            /**
             * Constructs a new EquipmentRecycleResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EquipmentRecycleResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EquipmentRecycleResult role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** EquipmentRecycleResult role_revision. */
            role_revision: Long;

            /** EquipmentRecycleResult source_seq. */
            source_seq: Long;

            /** EquipmentRecycleResult op_id. */
            op_id: Uint8Array;

            /** EquipmentRecycleResult policy_revision. */
            policy_revision: Long;

            /** EquipmentRecycleResult recycled_count. */
            recycled_count: Long;

            /** EquipmentRecycleResult gold. */
            gold: Long;

            /** EquipmentRecycleResult bound_gold. */
            bound_gold: Long;

            /** EquipmentRecycleResult discarded_count. */
            discarded_count: Long;

            /** EquipmentRecycleResult discard_reason. */
            discard_reason: string;

            /** EquipmentRecycleResult trigger. */
            trigger: number;

            /**
             * Creates a new EquipmentRecycleResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EquipmentRecycleResult instance
             */
            static create(properties: minimir.v1.EquipmentRecycleResult.$Shape): minimir.v1.EquipmentRecycleResult & minimir.v1.EquipmentRecycleResult.$Shape;
            static create(properties?: minimir.v1.EquipmentRecycleResult.$Properties): minimir.v1.EquipmentRecycleResult;

            /**
             * Encodes the specified EquipmentRecycleResult message. Does not implicitly {@link minimir.v1.EquipmentRecycleResult.verify|verify} messages.
             * @param message EquipmentRecycleResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EquipmentRecycleResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EquipmentRecycleResult message, length delimited. Does not implicitly {@link minimir.v1.EquipmentRecycleResult.verify|verify} messages.
             * @param message EquipmentRecycleResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EquipmentRecycleResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EquipmentRecycleResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EquipmentRecycleResult & minimir.v1.EquipmentRecycleResult.$Shape} EquipmentRecycleResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EquipmentRecycleResult & minimir.v1.EquipmentRecycleResult.$Shape;

            /**
             * Decodes an EquipmentRecycleResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EquipmentRecycleResult & minimir.v1.EquipmentRecycleResult.$Shape} EquipmentRecycleResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EquipmentRecycleResult & minimir.v1.EquipmentRecycleResult.$Shape;

            /**
             * Verifies an EquipmentRecycleResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EquipmentRecycleResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EquipmentRecycleResult {

            /** Properties of an EquipmentRecycleResult. */
            interface $Properties {

                /** EquipmentRecycleResult role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** EquipmentRecycleResult role_revision */
                role_revision?: (Long|null);

                /** EquipmentRecycleResult source_seq */
                source_seq?: (Long|null);

                /** EquipmentRecycleResult op_id */
                op_id?: (Uint8Array|null);

                /** EquipmentRecycleResult policy_revision */
                policy_revision?: (Long|null);

                /** EquipmentRecycleResult recycled_count */
                recycled_count?: (Long|null);

                /** EquipmentRecycleResult gold */
                gold?: (Long|null);

                /** EquipmentRecycleResult bound_gold */
                bound_gold?: (Long|null);

                /** EquipmentRecycleResult discarded_count */
                discarded_count?: (Long|null);

                /** EquipmentRecycleResult discard_reason */
                discard_reason?: (string|null);

                /** EquipmentRecycleResult trigger */
                trigger?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EquipmentRecycleResult. */
            type $Shape = minimir.v1.EquipmentRecycleResult.$Properties;
        }

        /**
         * Properties of an EquipRequest.
         * @deprecated Use minimir.v1.EquipRequest.$Properties instead.
         */
        interface IEquipRequest extends minimir.v1.EquipRequest.$Properties {
        }

        /** Represents an EquipRequest. */
        class EquipRequest {

            /**
             * Constructs a new EquipRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.EquipRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EquipRequest item_instance_id. */
            item_instance_id: Uint8Array;

            /** EquipRequest slot. */
            slot: number;

            /** EquipRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /** EquipRequest unequip. */
            unequip: boolean;

            /** EquipRequest item_version. */
            item_version: Long;

            /**
             * Creates a new EquipRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EquipRequest instance
             */
            static create(properties: minimir.v1.EquipRequest.$Shape): minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape;
            static create(properties?: minimir.v1.EquipRequest.$Properties): minimir.v1.EquipRequest;

            /**
             * Encodes the specified EquipRequest message. Does not implicitly {@link minimir.v1.EquipRequest.verify|verify} messages.
             * @param message EquipRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.EquipRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EquipRequest message, length delimited. Does not implicitly {@link minimir.v1.EquipRequest.verify|verify} messages.
             * @param message EquipRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.EquipRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EquipRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape} EquipRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape;

            /**
             * Decodes an EquipRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape} EquipRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape;

            /**
             * Verifies an EquipRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for EquipRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EquipRequest {

            /** Properties of an EquipRequest. */
            interface $Properties {

                /** EquipRequest item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** EquipRequest slot */
                slot?: (number|null);

                /** EquipRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** EquipRequest unequip */
                unequip?: (boolean|null);

                /** EquipRequest item_version */
                item_version?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EquipRequest. */
            type $Shape = minimir.v1.EquipRequest.$Properties;
        }

        /**
         * Properties of a RecycleRequest.
         * @deprecated Use minimir.v1.RecycleRequest.$Properties instead.
         */
        interface IRecycleRequest extends minimir.v1.RecycleRequest.$Properties {
        }

        /** Represents a RecycleRequest. */
        class RecycleRequest {

            /**
             * Constructs a new RecycleRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.RecycleRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** RecycleRequest item_instance_ids. */
            item_instance_ids: Uint8Array[];

            /** RecycleRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new RecycleRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns RecycleRequest instance
             */
            static create(properties: minimir.v1.RecycleRequest.$Shape): minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape;
            static create(properties?: minimir.v1.RecycleRequest.$Properties): minimir.v1.RecycleRequest;

            /**
             * Encodes the specified RecycleRequest message. Does not implicitly {@link minimir.v1.RecycleRequest.verify|verify} messages.
             * @param message RecycleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.RecycleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified RecycleRequest message, length delimited. Does not implicitly {@link minimir.v1.RecycleRequest.verify|verify} messages.
             * @param message RecycleRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.RecycleRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a RecycleRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape} RecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape;

            /**
             * Decodes a RecycleRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape} RecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape;

            /**
             * Verifies a RecycleRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for RecycleRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace RecycleRequest {

            /** Properties of a RecycleRequest. */
            interface $Properties {

                /** RecycleRequest item_instance_ids */
                item_instance_ids?: (Uint8Array[]|null);

                /** RecycleRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a RecycleRequest. */
            type $Shape = minimir.v1.RecycleRequest.$Properties;
        }

        /**
         * Properties of a UseItemRequest.
         * @deprecated Use minimir.v1.UseItemRequest.$Properties instead.
         */
        interface IUseItemRequest extends minimir.v1.UseItemRequest.$Properties {
        }

        /** Represents a UseItemRequest. */
        class UseItemRequest {

            /**
             * Constructs a new UseItemRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.UseItemRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** UseItemRequest item_instance_id. */
            item_instance_id: Uint8Array;

            /** UseItemRequest item_id. */
            item_id: number;

            /** UseItemRequest scene_epoch. */
            scene_epoch: Long;

            /** UseItemRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new UseItemRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns UseItemRequest instance
             */
            static create(properties: minimir.v1.UseItemRequest.$Shape): minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape;
            static create(properties?: minimir.v1.UseItemRequest.$Properties): minimir.v1.UseItemRequest;

            /**
             * Encodes the specified UseItemRequest message. Does not implicitly {@link minimir.v1.UseItemRequest.verify|verify} messages.
             * @param message UseItemRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.UseItemRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified UseItemRequest message, length delimited. Does not implicitly {@link minimir.v1.UseItemRequest.verify|verify} messages.
             * @param message UseItemRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.UseItemRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a UseItemRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape} UseItemRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape;

            /**
             * Decodes a UseItemRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape} UseItemRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape;

            /**
             * Verifies a UseItemRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for UseItemRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace UseItemRequest {

            /** Properties of a UseItemRequest. */
            interface $Properties {

                /** UseItemRequest item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** UseItemRequest item_id */
                item_id?: (number|null);

                /** UseItemRequest scene_epoch */
                scene_epoch?: (Long|null);

                /** UseItemRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a UseItemRequest. */
            type $Shape = minimir.v1.UseItemRequest.$Properties;
        }

        /**
         * Properties of a LearnSkillRequest.
         * @deprecated Use minimir.v1.LearnSkillRequest.$Properties instead.
         */
        interface ILearnSkillRequest extends minimir.v1.LearnSkillRequest.$Properties {
        }

        /** Represents a LearnSkillRequest. */
        class LearnSkillRequest {

            /**
             * Constructs a new LearnSkillRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.LearnSkillRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** LearnSkillRequest item_instance_id. */
            item_instance_id: Uint8Array;

            /** LearnSkillRequest skill_id. */
            skill_id: number;

            /** LearnSkillRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new LearnSkillRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns LearnSkillRequest instance
             */
            static create(properties: minimir.v1.LearnSkillRequest.$Shape): minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape;
            static create(properties?: minimir.v1.LearnSkillRequest.$Properties): minimir.v1.LearnSkillRequest;

            /**
             * Encodes the specified LearnSkillRequest message. Does not implicitly {@link minimir.v1.LearnSkillRequest.verify|verify} messages.
             * @param message LearnSkillRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.LearnSkillRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified LearnSkillRequest message, length delimited. Does not implicitly {@link minimir.v1.LearnSkillRequest.verify|verify} messages.
             * @param message LearnSkillRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.LearnSkillRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a LearnSkillRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape} LearnSkillRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape;

            /**
             * Decodes a LearnSkillRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape} LearnSkillRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape;

            /**
             * Verifies a LearnSkillRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for LearnSkillRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace LearnSkillRequest {

            /** Properties of a LearnSkillRequest. */
            interface $Properties {

                /** LearnSkillRequest item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** LearnSkillRequest skill_id */
                skill_id?: (number|null);

                /** LearnSkillRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a LearnSkillRequest. */
            type $Shape = minimir.v1.LearnSkillRequest.$Properties;
        }

        /**
         * Properties of a SetCastingSupplyRequest.
         * @deprecated Use minimir.v1.SetCastingSupplyRequest.$Properties instead.
         */
        interface ISetCastingSupplyRequest extends minimir.v1.SetCastingSupplyRequest.$Properties {
        }

        /** Represents a SetCastingSupplyRequest. */
        class SetCastingSupplyRequest {

            /**
             * Constructs a new SetCastingSupplyRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SetCastingSupplyRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SetCastingSupplyRequest item_instance_id. */
            item_instance_id: Uint8Array;

            /** SetCastingSupplyRequest item_version. */
            item_version: Long;

            /** SetCastingSupplyRequest unequip. */
            unequip: boolean;

            /** SetCastingSupplyRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new SetCastingSupplyRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SetCastingSupplyRequest instance
             */
            static create(properties: minimir.v1.SetCastingSupplyRequest.$Shape): minimir.v1.SetCastingSupplyRequest & minimir.v1.SetCastingSupplyRequest.$Shape;
            static create(properties?: minimir.v1.SetCastingSupplyRequest.$Properties): minimir.v1.SetCastingSupplyRequest;

            /**
             * Encodes the specified SetCastingSupplyRequest message. Does not implicitly {@link minimir.v1.SetCastingSupplyRequest.verify|verify} messages.
             * @param message SetCastingSupplyRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SetCastingSupplyRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SetCastingSupplyRequest message, length delimited. Does not implicitly {@link minimir.v1.SetCastingSupplyRequest.verify|verify} messages.
             * @param message SetCastingSupplyRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SetCastingSupplyRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SetCastingSupplyRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SetCastingSupplyRequest & minimir.v1.SetCastingSupplyRequest.$Shape} SetCastingSupplyRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SetCastingSupplyRequest & minimir.v1.SetCastingSupplyRequest.$Shape;

            /**
             * Decodes a SetCastingSupplyRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SetCastingSupplyRequest & minimir.v1.SetCastingSupplyRequest.$Shape} SetCastingSupplyRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SetCastingSupplyRequest & minimir.v1.SetCastingSupplyRequest.$Shape;

            /**
             * Verifies a SetCastingSupplyRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SetCastingSupplyRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SetCastingSupplyRequest {

            /** Properties of a SetCastingSupplyRequest. */
            interface $Properties {

                /** SetCastingSupplyRequest item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** SetCastingSupplyRequest item_version */
                item_version?: (Long|null);

                /** SetCastingSupplyRequest unequip */
                unequip?: (boolean|null);

                /** SetCastingSupplyRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SetCastingSupplyRequest. */
            type $Shape = minimir.v1.SetCastingSupplyRequest.$Properties;
        }

        /**
         * Properties of a CastWithMaterialRequest.
         * @deprecated Use minimir.v1.CastWithMaterialRequest.$Properties instead.
         */
        interface ICastWithMaterialRequest extends minimir.v1.CastWithMaterialRequest.$Properties {
        }

        /** Represents a CastWithMaterialRequest. */
        class CastWithMaterialRequest {

            /**
             * Constructs a new CastWithMaterialRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.CastWithMaterialRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CastWithMaterialRequest scene_epoch. */
            scene_epoch: Long;

            /** CastWithMaterialRequest input_sequence. */
            input_sequence: number;

            /** CastWithMaterialRequest skill_id. */
            skill_id: number;

            /** CastWithMaterialRequest target_handle. */
            target_handle: number;

            /** CastWithMaterialRequest target_grid. */
            target_grid?: (minimir.v1.GridPoint.$Properties|null);

            /** CastWithMaterialRequest direction. */
            direction: number;

            /** CastWithMaterialRequest material_instance_id. */
            material_instance_id: Uint8Array;

            /** CastWithMaterialRequest material_version. */
            material_version: Long;

            /** CastWithMaterialRequest caster_life. */
            caster_life: Long;

            /** CastWithMaterialRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new CastWithMaterialRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CastWithMaterialRequest instance
             */
            static create(properties: minimir.v1.CastWithMaterialRequest.$Shape): minimir.v1.CastWithMaterialRequest & minimir.v1.CastWithMaterialRequest.$Shape;
            static create(properties?: minimir.v1.CastWithMaterialRequest.$Properties): minimir.v1.CastWithMaterialRequest;

            /**
             * Encodes the specified CastWithMaterialRequest message. Does not implicitly {@link minimir.v1.CastWithMaterialRequest.verify|verify} messages.
             * @param message CastWithMaterialRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.CastWithMaterialRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CastWithMaterialRequest message, length delimited. Does not implicitly {@link minimir.v1.CastWithMaterialRequest.verify|verify} messages.
             * @param message CastWithMaterialRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.CastWithMaterialRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CastWithMaterialRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.CastWithMaterialRequest & minimir.v1.CastWithMaterialRequest.$Shape} CastWithMaterialRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.CastWithMaterialRequest & minimir.v1.CastWithMaterialRequest.$Shape;

            /**
             * Decodes a CastWithMaterialRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.CastWithMaterialRequest & minimir.v1.CastWithMaterialRequest.$Shape} CastWithMaterialRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.CastWithMaterialRequest & minimir.v1.CastWithMaterialRequest.$Shape;

            /**
             * Verifies a CastWithMaterialRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for CastWithMaterialRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CastWithMaterialRequest {

            /** Properties of a CastWithMaterialRequest. */
            interface $Properties {

                /** CastWithMaterialRequest scene_epoch */
                scene_epoch?: (Long|null);

                /** CastWithMaterialRequest input_sequence */
                input_sequence?: (number|null);

                /** CastWithMaterialRequest skill_id */
                skill_id?: (number|null);

                /** CastWithMaterialRequest target_handle */
                target_handle?: (number|null);

                /** CastWithMaterialRequest target_grid */
                target_grid?: (minimir.v1.GridPoint.$Properties|null);

                /** CastWithMaterialRequest direction */
                direction?: (number|null);

                /** CastWithMaterialRequest material_instance_id */
                material_instance_id?: (Uint8Array|null);

                /** CastWithMaterialRequest material_version */
                material_version?: (Long|null);

                /** CastWithMaterialRequest caster_life */
                caster_life?: (Long|null);

                /** CastWithMaterialRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CastWithMaterialRequest. */
            type $Shape = minimir.v1.CastWithMaterialRequest.$Properties;
        }

        /**
         * Properties of an ItemConsumption.
         * @deprecated Use minimir.v1.ItemConsumption.$Properties instead.
         */
        interface IItemConsumption extends minimir.v1.ItemConsumption.$Properties {
        }

        /** Represents an ItemConsumption. */
        class ItemConsumption {

            /**
             * Constructs a new ItemConsumption.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ItemConsumption.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ItemConsumption instance_id. */
            instance_id: Uint8Array;

            /** ItemConsumption version. */
            version: Long;

            /** ItemConsumption quantity. */
            quantity: Long;

            /**
             * Creates a new ItemConsumption instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ItemConsumption instance
             */
            static create(properties: minimir.v1.ItemConsumption.$Shape): minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape;
            static create(properties?: minimir.v1.ItemConsumption.$Properties): minimir.v1.ItemConsumption;

            /**
             * Encodes the specified ItemConsumption message. Does not implicitly {@link minimir.v1.ItemConsumption.verify|verify} messages.
             * @param message ItemConsumption message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ItemConsumption.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ItemConsumption message, length delimited. Does not implicitly {@link minimir.v1.ItemConsumption.verify|verify} messages.
             * @param message ItemConsumption message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ItemConsumption.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an ItemConsumption message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape} ItemConsumption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape;

            /**
             * Decodes an ItemConsumption message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape} ItemConsumption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape;

            /**
             * Verifies an ItemConsumption message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ItemConsumption
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ItemConsumption {

            /** Properties of an ItemConsumption. */
            interface $Properties {

                /** ItemConsumption instance_id */
                instance_id?: (Uint8Array|null);

                /** ItemConsumption version */
                version?: (Long|null);

                /** ItemConsumption quantity */
                quantity?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an ItemConsumption. */
            type $Shape = minimir.v1.ItemConsumption.$Properties;
        }

        /**
         * Properties of a SkillAssetPreviewRequest.
         * @deprecated Use minimir.v1.SkillAssetPreviewRequest.$Properties instead.
         */
        interface ISkillAssetPreviewRequest extends minimir.v1.SkillAssetPreviewRequest.$Properties {
        }

        /** Represents a SkillAssetPreviewRequest. */
        class SkillAssetPreviewRequest {

            /**
             * Constructs a new SkillAssetPreviewRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillAssetPreviewRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillAssetPreviewRequest kind. */
            kind: number;

            /** SkillAssetPreviewRequest offer_id. */
            offer_id: number;

            /** SkillAssetPreviewRequest quantity. */
            quantity: Long;

            /** SkillAssetPreviewRequest skill_id. */
            skill_id: number;

            /** SkillAssetPreviewRequest materials. */
            materials: minimir.v1.ItemConsumption.$Properties[];

            /**
             * Creates a new SkillAssetPreviewRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillAssetPreviewRequest instance
             */
            static create(properties: minimir.v1.SkillAssetPreviewRequest.$Shape): minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape;
            static create(properties?: minimir.v1.SkillAssetPreviewRequest.$Properties): minimir.v1.SkillAssetPreviewRequest;

            /**
             * Encodes the specified SkillAssetPreviewRequest message. Does not implicitly {@link minimir.v1.SkillAssetPreviewRequest.verify|verify} messages.
             * @param message SkillAssetPreviewRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillAssetPreviewRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillAssetPreviewRequest message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetPreviewRequest.verify|verify} messages.
             * @param message SkillAssetPreviewRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillAssetPreviewRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillAssetPreviewRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape} SkillAssetPreviewRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape;

            /**
             * Decodes a SkillAssetPreviewRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape} SkillAssetPreviewRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape;

            /**
             * Verifies a SkillAssetPreviewRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillAssetPreviewRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillAssetPreviewRequest {

            /** Properties of a SkillAssetPreviewRequest. */
            interface $Properties {

                /** SkillAssetPreviewRequest kind */
                kind?: (number|null);

                /** SkillAssetPreviewRequest offer_id */
                offer_id?: (number|null);

                /** SkillAssetPreviewRequest quantity */
                quantity?: (Long|null);

                /** SkillAssetPreviewRequest skill_id */
                skill_id?: (number|null);

                /** SkillAssetPreviewRequest materials */
                materials?: (minimir.v1.ItemConsumption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillAssetPreviewRequest. */
            type $Shape = minimir.v1.SkillAssetPreviewRequest.$Properties;
        }

        /**
         * Properties of a SkillMaterialView.
         * @deprecated Use minimir.v1.SkillMaterialView.$Properties instead.
         */
        interface ISkillMaterialView extends minimir.v1.SkillMaterialView.$Properties {
        }

        /** Represents a SkillMaterialView. */
        class SkillMaterialView {

            /**
             * Constructs a new SkillMaterialView.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillMaterialView.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillMaterialView item. */
            item?: (minimir.v1.ItemView.$Properties|null);

            /** SkillMaterialView consume_quantity. */
            consume_quantity: Long;

            /**
             * Creates a new SkillMaterialView instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillMaterialView instance
             */
            static create(properties: minimir.v1.SkillMaterialView.$Shape): minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape;
            static create(properties?: minimir.v1.SkillMaterialView.$Properties): minimir.v1.SkillMaterialView;

            /**
             * Encodes the specified SkillMaterialView message. Does not implicitly {@link minimir.v1.SkillMaterialView.verify|verify} messages.
             * @param message SkillMaterialView message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillMaterialView.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillMaterialView message, length delimited. Does not implicitly {@link minimir.v1.SkillMaterialView.verify|verify} messages.
             * @param message SkillMaterialView message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillMaterialView.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillMaterialView message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape} SkillMaterialView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape;

            /**
             * Decodes a SkillMaterialView message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape} SkillMaterialView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape;

            /**
             * Verifies a SkillMaterialView message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillMaterialView
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillMaterialView {

            /** Properties of a SkillMaterialView. */
            interface $Properties {

                /** SkillMaterialView item */
                item?: (minimir.v1.ItemView.$Properties|null);

                /** SkillMaterialView consume_quantity */
                consume_quantity?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillMaterialView. */
            type $Shape = minimir.v1.SkillMaterialView.$Properties;
        }

        /**
         * Properties of a SkillNumericComparison.
         * @deprecated Use minimir.v1.SkillNumericComparison.$Properties instead.
         */
        interface ISkillNumericComparison extends minimir.v1.SkillNumericComparison.$Properties {
        }

        /** Represents a SkillNumericComparison. */
        class SkillNumericComparison {

            /**
             * Constructs a new SkillNumericComparison.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillNumericComparison.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillNumericComparison key. */
            key: string;

            /** SkillNumericComparison before_min. */
            before_min: Long;

            /** SkillNumericComparison before_max. */
            before_max: Long;

            /** SkillNumericComparison after_min. */
            after_min: Long;

            /** SkillNumericComparison after_max. */
            after_max: Long;

            /**
             * Creates a new SkillNumericComparison instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillNumericComparison instance
             */
            static create(properties: minimir.v1.SkillNumericComparison.$Shape): minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape;
            static create(properties?: minimir.v1.SkillNumericComparison.$Properties): minimir.v1.SkillNumericComparison;

            /**
             * Encodes the specified SkillNumericComparison message. Does not implicitly {@link minimir.v1.SkillNumericComparison.verify|verify} messages.
             * @param message SkillNumericComparison message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillNumericComparison.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillNumericComparison message, length delimited. Does not implicitly {@link minimir.v1.SkillNumericComparison.verify|verify} messages.
             * @param message SkillNumericComparison message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillNumericComparison.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillNumericComparison message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape} SkillNumericComparison
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape;

            /**
             * Decodes a SkillNumericComparison message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape} SkillNumericComparison
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape;

            /**
             * Verifies a SkillNumericComparison message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillNumericComparison
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillNumericComparison {

            /** Properties of a SkillNumericComparison. */
            interface $Properties {

                /** SkillNumericComparison key */
                key?: (string|null);

                /** SkillNumericComparison before_min */
                before_min?: (Long|null);

                /** SkillNumericComparison before_max */
                before_max?: (Long|null);

                /** SkillNumericComparison after_min */
                after_min?: (Long|null);

                /** SkillNumericComparison after_max */
                after_max?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillNumericComparison. */
            type $Shape = minimir.v1.SkillNumericComparison.$Properties;
        }

        /**
         * Properties of a SkillAssetPreview.
         * @deprecated Use minimir.v1.SkillAssetPreview.$Properties instead.
         */
        interface ISkillAssetPreview extends minimir.v1.SkillAssetPreview.$Properties {
        }

        /** Represents a SkillAssetPreview. */
        class SkillAssetPreview {

            /**
             * Constructs a new SkillAssetPreview.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillAssetPreview.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillAssetPreview preview_id. */
            preview_id: Uint8Array;

            /** SkillAssetPreview kind. */
            kind: number;

            /** SkillAssetPreview rule_version. */
            rule_version: Long;

            /** SkillAssetPreview config_hash. */
            config_hash: string;

            /** SkillAssetPreview role_revision. */
            role_revision: Long;

            /** SkillAssetPreview expires_at_unix_ms. */
            expires_at_unix_ms: Long;

            /** SkillAssetPreview skill_id. */
            skill_id: number;

            /** SkillAssetPreview from_level. */
            from_level: number;

            /** SkillAssetPreview to_level. */
            to_level: number;

            /** SkillAssetPreview spend. */
            spend?: (minimir.v1.WalletState.$Properties|null);

            /** SkillAssetPreview materials. */
            materials: minimir.v1.SkillMaterialView.$Properties[];

            /** SkillAssetPreview outputs. */
            outputs: minimir.v1.ItemView.$Properties[];

            /** SkillAssetPreview stats. */
            stats: minimir.v1.SkillNumericComparison.$Properties[];

            /** SkillAssetPreview offer_id. */
            offer_id: number;

            /** SkillAssetPreview quantity. */
            quantity: Long;

            /**
             * Creates a new SkillAssetPreview instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillAssetPreview instance
             */
            static create(properties: minimir.v1.SkillAssetPreview.$Shape): minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape;
            static create(properties?: minimir.v1.SkillAssetPreview.$Properties): minimir.v1.SkillAssetPreview;

            /**
             * Encodes the specified SkillAssetPreview message. Does not implicitly {@link minimir.v1.SkillAssetPreview.verify|verify} messages.
             * @param message SkillAssetPreview message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillAssetPreview.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillAssetPreview message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetPreview.verify|verify} messages.
             * @param message SkillAssetPreview message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillAssetPreview.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillAssetPreview message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape} SkillAssetPreview
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape;

            /**
             * Decodes a SkillAssetPreview message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape} SkillAssetPreview
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape;

            /**
             * Verifies a SkillAssetPreview message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillAssetPreview
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillAssetPreview {

            /** Properties of a SkillAssetPreview. */
            interface $Properties {

                /** SkillAssetPreview preview_id */
                preview_id?: (Uint8Array|null);

                /** SkillAssetPreview kind */
                kind?: (number|null);

                /** SkillAssetPreview rule_version */
                rule_version?: (Long|null);

                /** SkillAssetPreview config_hash */
                config_hash?: (string|null);

                /** SkillAssetPreview role_revision */
                role_revision?: (Long|null);

                /** SkillAssetPreview expires_at_unix_ms */
                expires_at_unix_ms?: (Long|null);

                /** SkillAssetPreview skill_id */
                skill_id?: (number|null);

                /** SkillAssetPreview from_level */
                from_level?: (number|null);

                /** SkillAssetPreview to_level */
                to_level?: (number|null);

                /** SkillAssetPreview spend */
                spend?: (minimir.v1.WalletState.$Properties|null);

                /** SkillAssetPreview materials */
                materials?: (minimir.v1.SkillMaterialView.$Properties[]|null);

                /** SkillAssetPreview outputs */
                outputs?: (minimir.v1.ItemView.$Properties[]|null);

                /** SkillAssetPreview stats */
                stats?: (minimir.v1.SkillNumericComparison.$Properties[]|null);

                /** SkillAssetPreview offer_id */
                offer_id?: (number|null);

                /** SkillAssetPreview quantity */
                quantity?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillAssetPreview. */
            type $Shape = minimir.v1.SkillAssetPreview.$Properties;
        }

        /**
         * Properties of a SkillAssetCommitRequest.
         * @deprecated Use minimir.v1.SkillAssetCommitRequest.$Properties instead.
         */
        interface ISkillAssetCommitRequest extends minimir.v1.SkillAssetCommitRequest.$Properties {
        }

        /** Represents a SkillAssetCommitRequest. */
        class SkillAssetCommitRequest {

            /**
             * Constructs a new SkillAssetCommitRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.SkillAssetCommitRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SkillAssetCommitRequest preview_id. */
            preview_id: Uint8Array;

            /** SkillAssetCommitRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new SkillAssetCommitRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SkillAssetCommitRequest instance
             */
            static create(properties: minimir.v1.SkillAssetCommitRequest.$Shape): minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape;
            static create(properties?: minimir.v1.SkillAssetCommitRequest.$Properties): minimir.v1.SkillAssetCommitRequest;

            /**
             * Encodes the specified SkillAssetCommitRequest message. Does not implicitly {@link minimir.v1.SkillAssetCommitRequest.verify|verify} messages.
             * @param message SkillAssetCommitRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.SkillAssetCommitRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SkillAssetCommitRequest message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetCommitRequest.verify|verify} messages.
             * @param message SkillAssetCommitRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.SkillAssetCommitRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SkillAssetCommitRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape} SkillAssetCommitRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape;

            /**
             * Decodes a SkillAssetCommitRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape} SkillAssetCommitRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape;

            /**
             * Verifies a SkillAssetCommitRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for SkillAssetCommitRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SkillAssetCommitRequest {

            /** Properties of a SkillAssetCommitRequest. */
            interface $Properties {

                /** SkillAssetCommitRequest preview_id */
                preview_id?: (Uint8Array|null);

                /** SkillAssetCommitRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SkillAssetCommitRequest. */
            type $Shape = minimir.v1.SkillAssetCommitRequest.$Properties;
        }

        /**
         * Properties of a ClaimHostingTimeRequest.
         * @deprecated Use minimir.v1.ClaimHostingTimeRequest.$Properties instead.
         */
        interface IClaimHostingTimeRequest extends minimir.v1.ClaimHostingTimeRequest.$Properties {
        }

        /** Represents a ClaimHostingTimeRequest. */
        class ClaimHostingTimeRequest {

            /**
             * Constructs a new ClaimHostingTimeRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.ClaimHostingTimeRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ClaimHostingTimeRequest benefit_kind. */
            benefit_kind: number;

            /** ClaimHostingTimeRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new ClaimHostingTimeRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ClaimHostingTimeRequest instance
             */
            static create(properties: minimir.v1.ClaimHostingTimeRequest.$Shape): minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape;
            static create(properties?: minimir.v1.ClaimHostingTimeRequest.$Properties): minimir.v1.ClaimHostingTimeRequest;

            /**
             * Encodes the specified ClaimHostingTimeRequest message. Does not implicitly {@link minimir.v1.ClaimHostingTimeRequest.verify|verify} messages.
             * @param message ClaimHostingTimeRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.ClaimHostingTimeRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ClaimHostingTimeRequest message, length delimited. Does not implicitly {@link minimir.v1.ClaimHostingTimeRequest.verify|verify} messages.
             * @param message ClaimHostingTimeRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.ClaimHostingTimeRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ClaimHostingTimeRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape} ClaimHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape;

            /**
             * Decodes a ClaimHostingTimeRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape} ClaimHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape;

            /**
             * Verifies a ClaimHostingTimeRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for ClaimHostingTimeRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ClaimHostingTimeRequest {

            /** Properties of a ClaimHostingTimeRequest. */
            interface $Properties {

                /** ClaimHostingTimeRequest benefit_kind */
                benefit_kind?: (number|null);

                /** ClaimHostingTimeRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ClaimHostingTimeRequest. */
            type $Shape = minimir.v1.ClaimHostingTimeRequest.$Properties;
        }

        /**
         * Properties of a BuyHostingTimeRequest.
         * @deprecated Use minimir.v1.BuyHostingTimeRequest.$Properties instead.
         */
        interface IBuyHostingTimeRequest extends minimir.v1.BuyHostingTimeRequest.$Properties {
        }

        /** Represents a BuyHostingTimeRequest. */
        class BuyHostingTimeRequest {

            /**
             * Constructs a new BuyHostingTimeRequest.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.BuyHostingTimeRequest.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** BuyHostingTimeRequest requested_seconds. */
            requested_seconds: number;

            /** BuyHostingTimeRequest operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new BuyHostingTimeRequest instance using the specified properties.
             * @param [properties] Properties to set
             * @returns BuyHostingTimeRequest instance
             */
            static create(properties: minimir.v1.BuyHostingTimeRequest.$Shape): minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape;
            static create(properties?: minimir.v1.BuyHostingTimeRequest.$Properties): minimir.v1.BuyHostingTimeRequest;

            /**
             * Encodes the specified BuyHostingTimeRequest message. Does not implicitly {@link minimir.v1.BuyHostingTimeRequest.verify|verify} messages.
             * @param message BuyHostingTimeRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.BuyHostingTimeRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified BuyHostingTimeRequest message, length delimited. Does not implicitly {@link minimir.v1.BuyHostingTimeRequest.verify|verify} messages.
             * @param message BuyHostingTimeRequest message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.BuyHostingTimeRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a BuyHostingTimeRequest message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape} BuyHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape;

            /**
             * Decodes a BuyHostingTimeRequest message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape} BuyHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape;

            /**
             * Verifies a BuyHostingTimeRequest message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for BuyHostingTimeRequest
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace BuyHostingTimeRequest {

            /** Properties of a BuyHostingTimeRequest. */
            interface $Properties {

                /** BuyHostingTimeRequest requested_seconds */
                requested_seconds?: (number|null);

                /** BuyHostingTimeRequest operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a BuyHostingTimeRequest. */
            type $Shape = minimir.v1.BuyHostingTimeRequest.$Properties;
        }

        /**
         * Properties of a HostingReport.
         * @deprecated Use minimir.v1.HostingReport.$Properties instead.
         */
        interface IHostingReport extends minimir.v1.HostingReport.$Properties {
        }

        /** Represents a HostingReport. */
        class HostingReport {

            /**
             * Constructs a new HostingReport.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.HostingReport.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** HostingReport hosting_session_id. */
            hosting_session_id: Uint8Array;

            /** HostingReport used_seconds. */
            used_seconds: Long;

            /** HostingReport kills. */
            kills: Long;

            /** HostingReport experience. */
            experience: Long;

            /** HostingReport earned. */
            earned?: (minimir.v1.WalletState.$Properties|null);

            /** HostingReport drops. */
            drops: minimir.v1.ItemView.$Properties[];

            /** HostingReport stop_reason. */
            stop_reason: number;

            /** HostingReport settlement_sequence. */
            settlement_sequence: Long;

            /** HostingReport recycle_count. */
            recycle_count: Long;

            /** HostingReport recycle_gold. */
            recycle_gold: Long;

            /** HostingReport recycle_bound_gold. */
            recycle_bound_gold: Long;

            /** HostingReport discard_count. */
            discard_count: Long;

            /** HostingReport last_discard_reason. */
            last_discard_reason: string;

            /** HostingReport total_drop_count. */
            total_drop_count: Long;

            /** HostingReport raw_experience. */
            raw_experience: Long;

            /** HostingReport rejected_experience. */
            rejected_experience: Long;

            /** HostingReport pooled_experience. */
            pooled_experience: Long;

            /**
             * Creates a new HostingReport instance using the specified properties.
             * @param [properties] Properties to set
             * @returns HostingReport instance
             */
            static create(properties: minimir.v1.HostingReport.$Shape): minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape;
            static create(properties?: minimir.v1.HostingReport.$Properties): minimir.v1.HostingReport;

            /**
             * Encodes the specified HostingReport message. Does not implicitly {@link minimir.v1.HostingReport.verify|verify} messages.
             * @param message HostingReport message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.HostingReport.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified HostingReport message, length delimited. Does not implicitly {@link minimir.v1.HostingReport.verify|verify} messages.
             * @param message HostingReport message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.HostingReport.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a HostingReport message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape} HostingReport
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape;

            /**
             * Decodes a HostingReport message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape} HostingReport
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape;

            /**
             * Verifies a HostingReport message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for HostingReport
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace HostingReport {

            /** Properties of a HostingReport. */
            interface $Properties {

                /** HostingReport hosting_session_id */
                hosting_session_id?: (Uint8Array|null);

                /** HostingReport used_seconds */
                used_seconds?: (Long|null);

                /** HostingReport kills */
                kills?: (Long|null);

                /** HostingReport experience */
                experience?: (Long|null);

                /** HostingReport earned */
                earned?: (minimir.v1.WalletState.$Properties|null);

                /** HostingReport drops */
                drops?: (minimir.v1.ItemView.$Properties[]|null);

                /** HostingReport stop_reason */
                stop_reason?: (number|null);

                /** HostingReport settlement_sequence */
                settlement_sequence?: (Long|null);

                /** HostingReport recycle_count */
                recycle_count?: (Long|null);

                /** HostingReport recycle_gold */
                recycle_gold?: (Long|null);

                /** HostingReport recycle_bound_gold */
                recycle_bound_gold?: (Long|null);

                /** HostingReport discard_count */
                discard_count?: (Long|null);

                /** HostingReport last_discard_reason */
                last_discard_reason?: (string|null);

                /** HostingReport total_drop_count */
                total_drop_count?: (Long|null);

                /** HostingReport raw_experience */
                raw_experience?: (Long|null);

                /** HostingReport rejected_experience */
                rejected_experience?: (Long|null);

                /** HostingReport pooled_experience */
                pooled_experience?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a HostingReport. */
            type $Shape = minimir.v1.HostingReport.$Properties;
        }

        /**
         * Properties of a WarehouseQuery.
         * @deprecated Use minimir.v1.WarehouseQuery.$Properties instead.
         */
        interface IWarehouseQuery extends minimir.v1.WarehouseQuery.$Properties {
        }

        /** Represents a WarehouseQuery. */
        class WarehouseQuery {

            /**
             * Constructs a new WarehouseQuery.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehouseQuery.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehouseQuery scope. */
            scope: number;

            /** WarehouseQuery start_slot. */
            start_slot: Long;

            /** WarehouseQuery limit. */
            limit: number;

            /**
             * Creates a new WarehouseQuery instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehouseQuery instance
             */
            static create(properties: minimir.v1.WarehouseQuery.$Shape): minimir.v1.WarehouseQuery & minimir.v1.WarehouseQuery.$Shape;
            static create(properties?: minimir.v1.WarehouseQuery.$Properties): minimir.v1.WarehouseQuery;

            /**
             * Encodes the specified WarehouseQuery message. Does not implicitly {@link minimir.v1.WarehouseQuery.verify|verify} messages.
             * @param message WarehouseQuery message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehouseQuery.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehouseQuery message, length delimited. Does not implicitly {@link minimir.v1.WarehouseQuery.verify|verify} messages.
             * @param message WarehouseQuery message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehouseQuery.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehouseQuery message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehouseQuery & minimir.v1.WarehouseQuery.$Shape} WarehouseQuery
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehouseQuery & minimir.v1.WarehouseQuery.$Shape;

            /**
             * Decodes a WarehouseQuery message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehouseQuery & minimir.v1.WarehouseQuery.$Shape} WarehouseQuery
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehouseQuery & minimir.v1.WarehouseQuery.$Shape;

            /**
             * Verifies a WarehouseQuery message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehouseQuery
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehouseQuery {

            /** Properties of a WarehouseQuery. */
            interface $Properties {

                /** WarehouseQuery scope */
                scope?: (number|null);

                /** WarehouseQuery start_slot */
                start_slot?: (Long|null);

                /** WarehouseQuery limit */
                limit?: (number|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehouseQuery. */
            type $Shape = minimir.v1.WarehouseQuery.$Properties;
        }

        /**
         * Properties of a WarehouseItem.
         * @deprecated Use minimir.v1.WarehouseItem.$Properties instead.
         */
        interface IWarehouseItem extends minimir.v1.WarehouseItem.$Properties {
        }

        /** Represents a WarehouseItem. */
        class WarehouseItem {

            /**
             * Constructs a new WarehouseItem.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehouseItem.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehouseItem slot. */
            slot: Long;

            /** WarehouseItem item. */
            item?: (minimir.v1.ItemView.$Properties|null);

            /**
             * Creates a new WarehouseItem instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehouseItem instance
             */
            static create(properties: minimir.v1.WarehouseItem.$Shape): minimir.v1.WarehouseItem & minimir.v1.WarehouseItem.$Shape;
            static create(properties?: minimir.v1.WarehouseItem.$Properties): minimir.v1.WarehouseItem;

            /**
             * Encodes the specified WarehouseItem message. Does not implicitly {@link minimir.v1.WarehouseItem.verify|verify} messages.
             * @param message WarehouseItem message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehouseItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehouseItem message, length delimited. Does not implicitly {@link minimir.v1.WarehouseItem.verify|verify} messages.
             * @param message WarehouseItem message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehouseItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehouseItem message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehouseItem & minimir.v1.WarehouseItem.$Shape} WarehouseItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehouseItem & minimir.v1.WarehouseItem.$Shape;

            /**
             * Decodes a WarehouseItem message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehouseItem & minimir.v1.WarehouseItem.$Shape} WarehouseItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehouseItem & minimir.v1.WarehouseItem.$Shape;

            /**
             * Verifies a WarehouseItem message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehouseItem
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehouseItem {

            /** Properties of a WarehouseItem. */
            interface $Properties {

                /** WarehouseItem slot */
                slot?: (Long|null);

                /** WarehouseItem item */
                item?: (minimir.v1.ItemView.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehouseItem. */
            type $Shape = minimir.v1.WarehouseItem.$Properties;
        }

        /**
         * Properties of a WarehousePage.
         * @deprecated Use minimir.v1.WarehousePage.$Properties instead.
         */
        interface IWarehousePage extends minimir.v1.WarehousePage.$Properties {
        }

        /** Represents a WarehousePage. */
        class WarehousePage {

            /**
             * Constructs a new WarehousePage.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehousePage.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehousePage role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** WarehousePage scope. */
            scope: number;

            /** WarehousePage revision. */
            revision: Long;

            /** WarehousePage capacity. */
            capacity: Long;

            /** WarehousePage start_slot. */
            start_slot: Long;

            /** WarehousePage limit. */
            limit: number;

            /** WarehousePage items. */
            items: minimir.v1.WarehouseItem.$Properties[];

            /** WarehousePage gold. */
            gold: Long;

            /** WarehousePage yuanbao_units. */
            yuanbao_units: Long;

            /**
             * Creates a new WarehousePage instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehousePage instance
             */
            static create(properties: minimir.v1.WarehousePage.$Shape): minimir.v1.WarehousePage & minimir.v1.WarehousePage.$Shape;
            static create(properties?: minimir.v1.WarehousePage.$Properties): minimir.v1.WarehousePage;

            /**
             * Encodes the specified WarehousePage message. Does not implicitly {@link minimir.v1.WarehousePage.verify|verify} messages.
             * @param message WarehousePage message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehousePage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehousePage message, length delimited. Does not implicitly {@link minimir.v1.WarehousePage.verify|verify} messages.
             * @param message WarehousePage message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehousePage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehousePage message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehousePage & minimir.v1.WarehousePage.$Shape} WarehousePage
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehousePage & minimir.v1.WarehousePage.$Shape;

            /**
             * Decodes a WarehousePage message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehousePage & minimir.v1.WarehousePage.$Shape} WarehousePage
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehousePage & minimir.v1.WarehousePage.$Shape;

            /**
             * Verifies a WarehousePage message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehousePage
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehousePage {

            /** Properties of a WarehousePage. */
            interface $Properties {

                /** WarehousePage role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** WarehousePage scope */
                scope?: (number|null);

                /** WarehousePage revision */
                revision?: (Long|null);

                /** WarehousePage capacity */
                capacity?: (Long|null);

                /** WarehousePage start_slot */
                start_slot?: (Long|null);

                /** WarehousePage limit */
                limit?: (number|null);

                /** WarehousePage items */
                items?: (minimir.v1.WarehouseItem.$Properties[]|null);

                /** WarehousePage gold */
                gold?: (Long|null);

                /** WarehousePage yuanbao_units */
                yuanbao_units?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehousePage. */
            type $Shape = minimir.v1.WarehousePage.$Properties;
        }

        /**
         * Properties of a WarehouseAction.
         * @deprecated Use minimir.v1.WarehouseAction.$Properties instead.
         */
        interface IWarehouseAction extends minimir.v1.WarehouseAction.$Properties {
        }

        /** Represents a WarehouseAction. */
        class WarehouseAction {

            /**
             * Constructs a new WarehouseAction.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehouseAction.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehouseAction scope. */
            scope: number;

            /** WarehouseAction action. */
            action: number;

            /** WarehouseAction warehouse_revision. */
            warehouse_revision: Long;

            /** WarehouseAction warehouse_slot. */
            warehouse_slot: Long;

            /** WarehouseAction item_instance_id. */
            item_instance_id: Uint8Array;

            /** WarehouseAction item_version. */
            item_version: Long;

            /** WarehouseAction quantity. */
            quantity: Long;

            /** WarehouseAction currency. */
            currency: number;

            /** WarehouseAction amount. */
            amount: Long;

            /** WarehouseAction operation. */
            operation?: (minimir.v1.OperationContext.$Properties|null);

            /**
             * Creates a new WarehouseAction instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehouseAction instance
             */
            static create(properties: minimir.v1.WarehouseAction.$Shape): minimir.v1.WarehouseAction & minimir.v1.WarehouseAction.$Shape;
            static create(properties?: minimir.v1.WarehouseAction.$Properties): minimir.v1.WarehouseAction;

            /**
             * Encodes the specified WarehouseAction message. Does not implicitly {@link minimir.v1.WarehouseAction.verify|verify} messages.
             * @param message WarehouseAction message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehouseAction.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehouseAction message, length delimited. Does not implicitly {@link minimir.v1.WarehouseAction.verify|verify} messages.
             * @param message WarehouseAction message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehouseAction.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehouseAction message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehouseAction & minimir.v1.WarehouseAction.$Shape} WarehouseAction
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehouseAction & minimir.v1.WarehouseAction.$Shape;

            /**
             * Decodes a WarehouseAction message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehouseAction & minimir.v1.WarehouseAction.$Shape} WarehouseAction
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehouseAction & minimir.v1.WarehouseAction.$Shape;

            /**
             * Verifies a WarehouseAction message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehouseAction
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehouseAction {

            /** Properties of a WarehouseAction. */
            interface $Properties {

                /** WarehouseAction scope */
                scope?: (number|null);

                /** WarehouseAction action */
                action?: (number|null);

                /** WarehouseAction warehouse_revision */
                warehouse_revision?: (Long|null);

                /** WarehouseAction warehouse_slot */
                warehouse_slot?: (Long|null);

                /** WarehouseAction item_instance_id */
                item_instance_id?: (Uint8Array|null);

                /** WarehouseAction item_version */
                item_version?: (Long|null);

                /** WarehouseAction quantity */
                quantity?: (Long|null);

                /** WarehouseAction currency */
                currency?: (number|null);

                /** WarehouseAction amount */
                amount?: (Long|null);

                /** WarehouseAction operation */
                operation?: (minimir.v1.OperationContext.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehouseAction. */
            type $Shape = minimir.v1.WarehouseAction.$Properties;
        }

        /**
         * Properties of a WarehouseItemChange.
         * @deprecated Use minimir.v1.WarehouseItemChange.$Properties instead.
         */
        interface IWarehouseItemChange extends minimir.v1.WarehouseItemChange.$Properties {
        }

        /** Represents a WarehouseItemChange. */
        class WarehouseItemChange {

            /**
             * Constructs a new WarehouseItemChange.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehouseItemChange.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehouseItemChange slot. */
            slot: Long;

            /** WarehouseItemChange before. */
            before?: (minimir.v1.ItemView.$Properties|null);

            /** WarehouseItemChange after. */
            after?: (minimir.v1.ItemView.$Properties|null);

            /**
             * Creates a new WarehouseItemChange instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehouseItemChange instance
             */
            static create(properties: minimir.v1.WarehouseItemChange.$Shape): minimir.v1.WarehouseItemChange & minimir.v1.WarehouseItemChange.$Shape;
            static create(properties?: minimir.v1.WarehouseItemChange.$Properties): minimir.v1.WarehouseItemChange;

            /**
             * Encodes the specified WarehouseItemChange message. Does not implicitly {@link minimir.v1.WarehouseItemChange.verify|verify} messages.
             * @param message WarehouseItemChange message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehouseItemChange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehouseItemChange message, length delimited. Does not implicitly {@link minimir.v1.WarehouseItemChange.verify|verify} messages.
             * @param message WarehouseItemChange message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehouseItemChange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehouseItemChange message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehouseItemChange & minimir.v1.WarehouseItemChange.$Shape} WarehouseItemChange
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehouseItemChange & minimir.v1.WarehouseItemChange.$Shape;

            /**
             * Decodes a WarehouseItemChange message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehouseItemChange & minimir.v1.WarehouseItemChange.$Shape} WarehouseItemChange
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehouseItemChange & minimir.v1.WarehouseItemChange.$Shape;

            /**
             * Verifies a WarehouseItemChange message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehouseItemChange
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehouseItemChange {

            /** Properties of a WarehouseItemChange. */
            interface $Properties {

                /** WarehouseItemChange slot */
                slot?: (Long|null);

                /** WarehouseItemChange before */
                before?: (minimir.v1.ItemView.$Properties|null);

                /** WarehouseItemChange after */
                after?: (minimir.v1.ItemView.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehouseItemChange. */
            type $Shape = minimir.v1.WarehouseItemChange.$Properties;
        }

        /**
         * Properties of a WarehouseActionResult.
         * @deprecated Use minimir.v1.WarehouseActionResult.$Properties instead.
         */
        interface IWarehouseActionResult extends minimir.v1.WarehouseActionResult.$Properties {
        }

        /** Represents a WarehouseActionResult. */
        class WarehouseActionResult {

            /**
             * Constructs a new WarehouseActionResult.
             * @param [properties] Properties to set
             */
            constructor(properties?: minimir.v1.WarehouseActionResult.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** WarehouseActionResult role. */
            role?: (minimir.v1.RoleKey.$Properties|null);

            /** WarehouseActionResult role_revision. */
            role_revision: Long;

            /** WarehouseActionResult op_id. */
            op_id: Uint8Array;

            /** WarehouseActionResult scope. */
            scope: number;

            /** WarehouseActionResult action. */
            action: number;

            /** WarehouseActionResult warehouse_revision. */
            warehouse_revision: Long;

            /** WarehouseActionResult capacity. */
            capacity: Long;

            /** WarehouseActionResult changes. */
            changes: minimir.v1.WarehouseItemChange.$Properties[];

            /** WarehouseActionResult gold. */
            gold: Long;

            /** WarehouseActionResult yuanbao_units. */
            yuanbao_units: Long;

            /**
             * Creates a new WarehouseActionResult instance using the specified properties.
             * @param [properties] Properties to set
             * @returns WarehouseActionResult instance
             */
            static create(properties: minimir.v1.WarehouseActionResult.$Shape): minimir.v1.WarehouseActionResult & minimir.v1.WarehouseActionResult.$Shape;
            static create(properties?: minimir.v1.WarehouseActionResult.$Properties): minimir.v1.WarehouseActionResult;

            /**
             * Encodes the specified WarehouseActionResult message. Does not implicitly {@link minimir.v1.WarehouseActionResult.verify|verify} messages.
             * @param message WarehouseActionResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: minimir.v1.WarehouseActionResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified WarehouseActionResult message, length delimited. Does not implicitly {@link minimir.v1.WarehouseActionResult.verify|verify} messages.
             * @param message WarehouseActionResult message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: minimir.v1.WarehouseActionResult.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a WarehouseActionResult message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {minimir.v1.WarehouseActionResult & minimir.v1.WarehouseActionResult.$Shape} WarehouseActionResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): minimir.v1.WarehouseActionResult & minimir.v1.WarehouseActionResult.$Shape;

            /**
             * Decodes a WarehouseActionResult message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {minimir.v1.WarehouseActionResult & minimir.v1.WarehouseActionResult.$Shape} WarehouseActionResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): minimir.v1.WarehouseActionResult & minimir.v1.WarehouseActionResult.$Shape;

            /**
             * Verifies a WarehouseActionResult message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Gets the type url for WarehouseActionResult
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace WarehouseActionResult {

            /** Properties of a WarehouseActionResult. */
            interface $Properties {

                /** WarehouseActionResult role */
                role?: (minimir.v1.RoleKey.$Properties|null);

                /** WarehouseActionResult role_revision */
                role_revision?: (Long|null);

                /** WarehouseActionResult op_id */
                op_id?: (Uint8Array|null);

                /** WarehouseActionResult scope */
                scope?: (number|null);

                /** WarehouseActionResult action */
                action?: (number|null);

                /** WarehouseActionResult warehouse_revision */
                warehouse_revision?: (Long|null);

                /** WarehouseActionResult capacity */
                capacity?: (Long|null);

                /** WarehouseActionResult changes */
                changes?: (minimir.v1.WarehouseItemChange.$Properties[]|null);

                /** WarehouseActionResult gold */
                gold?: (Long|null);

                /** WarehouseActionResult yuanbao_units */
                yuanbao_units?: (Long|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a WarehouseActionResult. */
            type $Shape = minimir.v1.WarehouseActionResult.$Properties;
        }
    }
}
