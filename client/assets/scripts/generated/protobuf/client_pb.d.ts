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

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EntityState. */
            type $Shape = minimir.v1.EntityState.$Properties;
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

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an InventoryResult. */
            type $Shape = minimir.v1.InventoryResult.$Properties;
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

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a HostingReport. */
            type $Shape = minimir.v1.HostingReport.$Properties;
        }
    }
}
